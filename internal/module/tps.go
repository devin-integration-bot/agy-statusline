package module

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mritd/agy-statusline/internal/ansi"
	"github.com/mritd/agy-statusline/internal/config"
	"github.com/mritd/agy-statusline/internal/debug"
	"github.com/mritd/agy-statusline/internal/stdin"
)

const (
	minTPSDuration     = 100 * time.Millisecond
	maxTPSDuration     = 10 * time.Minute
	pendingGraceWindow = 5 * time.Second
	stateFileMaxAge    = 7 * 24 * time.Hour
	maxSessionIDLen    = 128
)

// TPSModule tracks model output throughput (tokens per second) per conversation.
// Primary source: live cross-invocation state stored in a per-session temp file
// (millisecond precision, covers both tool-calling and text-only responses).
// Fallback: PLANNER_RESPONSE -> GENERIC step pairs parsed from transcript.jsonl
// for immediate values on cold start or resumed sessions.
type TPSModule struct {
	stateDir string
	now      func() time.Time

	tps      float64
	avg      float64
	tokens   int
	duration time.Duration
}

type tpsState struct {
	LastSeenMs      int64 `json:"last_seen_ms"`
	LastTotalOutput int   `json:"last_total_output"`

	// Active model generation window (while agent_state is working/thinking).
	WorkingStartMs int64 `json:"working_start_ms,omitempty"`
	WorkingBaseOut int   `json:"working_base_out,omitempty"`

	// Grace window when agent_state leaves working before total_output_tokens flushes.
	PendingStartMs int64 `json:"pending_start_ms,omitempty"`
	PendingEndMs   int64 `json:"pending_end_ms,omitempty"`
	PendingBaseOut int   `json:"pending_base_out,omitempty"`

	// Most recent finalized step measurement.
	LastTPS        float64 `json:"last_tps,omitempty"`
	LastTokens     int     `json:"last_tokens,omitempty"`
	LastDurationMs int64   `json:"last_duration_ms,omitempty"`

	// Cumulative finalized steps in this conversation.
	SessionTokens int   `json:"session_tokens,omitempty"`
	SessionDurMs  int64 `json:"session_dur_ms,omitempty"`
}

func NewTPSModule() *TPSModule {
	return &TPSModule{
		now: time.Now,
	}
}

func (m *TPSModule) Name() string { return "tps" }

func (m *TPSModule) Collect(ctx *Context) error {
	if ctx == nil || ctx.Stdin == nil {
		return nil
	}

	if sessionID := resolveSessionID(ctx.Stdin); isSafeSessionID(sessionID) {
		m.collectLive(ctx.Stdin, sessionID)
	}

	// Fall back to transcript-derived TPS when live state has no measurement yet
	// (e.g. cold start, resumed session, or stateless invocation).
	if m.tps <= 0 {
		if td := ctx.LoadTranscript(); td != nil && td.LastTPS > 0 {
			m.tps = td.LastTPS
			m.avg = td.AvgTPS
			m.tokens = td.LastOutputTokens
			m.duration = td.LastDuration
		}
	}

	return nil
}

func (m *TPSModule) Vars(ctx *Context) map[string]string {
	if m.tps <= 0 || math.IsNaN(m.tps) || math.IsInf(m.tps, 0) {
		return nil
	}

	rateStr := formatRate(m.tps)
	avgRateStr := ""
	avgStr := ""
	if m.avg > 0 && !math.IsNaN(m.avg) && !math.IsInf(m.avg, 0) {
		avgRateStr = formatRate(m.avg)
		avgStr = avgRateStr + " tok/s"
	}

	tokensStr := ""
	if m.tokens > 0 {
		tokensStr = formatStepTokens(m.tokens)
	}

	durStr := ""
	if m.duration > 0 {
		durStr = formatStepDuration(m.duration)
	}

	return map[string]string{
		"tps":      ansi.Colored(ansi.Cyan, rateStr+" tok/s"),
		"rate":     rateStr,
		"avg":      avgStr,
		"avg_rate": avgRateStr,
		"tokens":   tokensStr,
		"duration": durStr,
	}
}

func (m *TPSModule) DefaultFormat() string {
	return "{tps}"
}

func (m *TPSModule) collectLive(in *stdin.Data, sessionID string) {
	nowFn := m.now
	if nowFn == nil {
		nowFn = time.Now
	}
	nowMs := nowFn().UnixMilli()

	path := m.stateFilePath(sessionID)
	st, existed := loadTPSState(path)
	prev := st

	totalOut := max(0, in.ContextWindow.TotalOutputTokens)
	isWorking := isWorkingState(in.AgentState) && !in.ToolConfirmation

	// Guard against counter regressions (e.g. rewound session).
	if totalOut < st.LastTotalOutput {
		st.clearWorking()
		st.clearPending()
	}

	// Expire stale timers caused by sleep, crashes, or long idle gaps.
	if st.WorkingStartMs > 0 {
		elapsed := nowMs - st.WorkingStartMs
		if elapsed < 0 || elapsed > maxTPSDuration.Milliseconds() {
			st.clearWorking()
		}
	}
	if st.PendingStartMs > 0 {
		sinceEnd := nowMs - st.PendingEndMs
		if sinceEnd < 0 || sinceEnd > pendingGraceWindow.Milliseconds() || st.PendingEndMs < st.PendingStartMs {
			st.clearPending()
		}
	}

	var liveTPS float64
	var liveTokens int
	var liveDurMs int64

	if isWorking {
		if st.WorkingStartMs == 0 {
			// If tokens from the previous step just arrived as a new step begins,
			// finalize the previous step first.
			if st.PendingStartMs > 0 && totalOut > st.PendingBaseOut {
				durMs := resolvePendingDuration(st.PendingStartMs, st.PendingEndMs, nowMs)
				st.recordStep(totalOut-st.PendingBaseOut, durMs)
			}
			st.clearPending()
			st.WorkingStartMs = nowMs
			st.WorkingBaseOut = totalOut
		} else if delta := totalOut - st.WorkingBaseOut; delta > 0 {
			// Streaming update while still working: compute live rate without
			// double-counting into cumulative session totals until finalized.
			durMs := nowMs - st.WorkingStartMs
			if isValidDuration(durMs) {
				liveTPS = computeTPS(delta, durMs)
				liveTokens = delta
				liveDurMs = durMs
			}
		}
	} else {
		if st.WorkingStartMs > 0 {
			delta := totalOut - st.WorkingBaseOut
			durMs := nowMs - st.WorkingStartMs
			if delta > 0 {
				st.recordStep(delta, durMs)
				st.clearPending()
			} else {
				// Token count has not flushed yet; keep a short grace window.
				st.PendingStartMs = st.WorkingStartMs
				st.PendingEndMs = nowMs
				st.PendingBaseOut = st.WorkingBaseOut
			}
			st.clearWorking()
		} else if st.PendingStartMs > 0 && totalOut > st.PendingBaseOut {
			durMs := resolvePendingDuration(st.PendingStartMs, st.PendingEndMs, nowMs)
			st.recordStep(totalOut-st.PendingBaseOut, durMs)
			st.clearPending()
		}
	}

	st.LastTotalOutput = totalOut
	st.LastSeenMs = nowMs

	// Only write when meaningful state changed (or initial active state needs recording).
	if tpsStateChanged(prev, st) || (!existed && (isWorking || totalOut > 0)) {
		if err := saveTPSState(path, st); err != nil {
			debug.Log("tps", "save state %s: %v", path, err)
		} else if !existed {
			pruneStaleTPSState(filepath.Dir(path), nowFn())
		}
	}

	if liveTPS > 0 {
		m.tps = liveTPS
		m.tokens = liveTokens
		m.duration = time.Duration(liveDurMs) * time.Millisecond
		totTokens := st.SessionTokens + liveTokens
		totDurMs := st.SessionDurMs + liveDurMs
		if totTokens > 0 && totDurMs > 0 {
			m.avg = computeTPS(totTokens, totDurMs)
		}
		return
	}

	if st.LastTPS > 0 {
		m.tps = st.LastTPS
		m.tokens = st.LastTokens
		m.duration = time.Duration(st.LastDurationMs) * time.Millisecond
		if st.SessionTokens > 0 && st.SessionDurMs > 0 {
			m.avg = computeTPS(st.SessionTokens, st.SessionDurMs)
		}
	}
}

func (s *tpsState) clearWorking() {
	s.WorkingStartMs = 0
	s.WorkingBaseOut = 0
}

func (s *tpsState) clearPending() {
	s.PendingStartMs = 0
	s.PendingEndMs = 0
	s.PendingBaseOut = 0
}

func (s *tpsState) recordStep(delta int, durMs int64) {
	if delta <= 0 || !isValidDuration(durMs) {
		return
	}
	s.LastTPS = computeTPS(delta, durMs)
	s.LastTokens = delta
	s.LastDurationMs = durMs
	s.SessionTokens += delta
	s.SessionDurMs += durMs
}

func resolvePendingDuration(startMs, endMs, nowMs int64) int64 {
	durMs := endMs - startMs
	if durMs < minTPSDuration.Milliseconds() {
		durMs = nowMs - startMs
	}
	return durMs
}

func isValidDuration(durMs int64) bool {
	return durMs >= minTPSDuration.Milliseconds() && durMs <= maxTPSDuration.Milliseconds()
}

func computeTPS(tokens int, durMs int64) float64 {
	return float64(tokens) / (float64(durMs) / 1000.0)
}

func isWorkingState(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "working", "thinking":
		return true
	default:
		return false
	}
}

func tpsStateChanged(a, b tpsState) bool {
	return a.LastTotalOutput != b.LastTotalOutput ||
		a.WorkingStartMs != b.WorkingStartMs ||
		a.WorkingBaseOut != b.WorkingBaseOut ||
		a.PendingStartMs != b.PendingStartMs ||
		a.PendingEndMs != b.PendingEndMs ||
		a.PendingBaseOut != b.PendingBaseOut ||
		a.LastTPS != b.LastTPS ||
		a.LastTokens != b.LastTokens ||
		a.LastDurationMs != b.LastDurationMs ||
		a.SessionTokens != b.SessionTokens ||
		a.SessionDurMs != b.SessionDurMs
}

func resolveSessionID(in *stdin.Data) string {
	if in == nil {
		return ""
	}
	if id := strings.TrimSpace(in.ConversationID); id != "" {
		return id
	}
	return strings.TrimSpace(in.SessionID)
}

func isSafeSessionID(id string) bool {
	if id == "" || len(id) > maxSessionIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			continue
		}
		return false
	}
	return true
}

func (m *TPSModule) stateFilePath(sessionID string) string {
	dir := m.stateDir
	if dir == "" {
		dir = defaultTPSStateDir()
	}
	return filepath.Join(dir, sessionID+".json")
}

func defaultTPSStateDir() string {
	return filepath.Join(config.PluginDir(), "cache", "tps")
}

func pruneStaleTPSState(dir string, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := now.Add(-stateFileMaxAge)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

func loadTPSState(path string) (tpsState, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return tpsState{}, false
	}
	var st tpsState
	if err := json.Unmarshal(data, &st); err != nil {
		debug.Log("tps", "corrupt state %s: %v", path, err)
		return tpsState{}, false
	}
	return st, true
}

func saveTPSState(path string, st tpsState) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".tps-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func formatRate(v float64) string {
	return fmt.Sprintf("%.1f", v)
}

func formatStepTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func formatStepDuration(d time.Duration) string {
	if d < 100*time.Millisecond {
		return "<0.1s"
	}
	if d < 59950*time.Millisecond {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	totalSecs := int(math.Round(d.Seconds()))
	mins := totalSecs / 60
	secs := totalSecs % 60
	return fmt.Sprintf("%dm %ds", mins, secs)
}
