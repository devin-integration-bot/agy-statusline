package module

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mritd/agy-statusline/internal/ansi"
	"github.com/mritd/agy-statusline/internal/config"
	"github.com/mritd/agy-statusline/internal/debug"
	"github.com/mritd/agy-statusline/internal/stdin"
	"github.com/mritd/agy-statusline/internal/transcript"
)

const (
	minTPSDuration       = 100 * time.Millisecond
	maxTPSDuration       = 10 * time.Minute
	pendingGraceWindow   = 5 * time.Second
	stateFileMaxAge      = 7 * 24 * time.Hour
	maxSessionIDLen      = 128
	minSubstantialTokens = 1000 // short steps (<1k tok) are TTFT-dominated and won't overwrite primary {tps}
	maxSavedTextSteps    = 32
	minOLSSamples        = 5
	minOLSTokenSpread    = 1500
	minOLSR2             = 0.5
	maxOLSDecodeTPS      = 2000.0
	maxOLSTTFTSec        = 120.0
)

// TPSModule tracks model output throughput (tokens per second) per conversation.
// In hybrid mode (when transcript.jsonl has planner steps), tool-calling steps
// use exact PLANNER_RESPONSE -> last GENERIC boundaries (excluding tool runtime),
// while text-only final replies anchor to LatestTextStep.CreatedAt.
// Short steps (<1000 tokens) still feed {avg} and OLS {decode}/{ttft} without
// overwriting primary {tps} once a substantial step exists for the active model.
type TPSModule struct {
	stateDir string
	now      func() time.Time

	tps      float64
	avg      float64
	decode   float64
	ttft     time.Duration
	hasTTFT  bool
	tokens   int
	duration time.Duration
}

type tpsStepRecord struct {
	StepIndex int   `json:"step_index"`
	Tokens    int   `json:"tokens"`
	DurMs     int64 `json:"dur_ms"`
}

type tpsState struct {
	ActiveModel     string `json:"active_model,omitempty"`
	LastSeenMs      int64  `json:"last_seen_ms"`
	LastTotalOutput int    `json:"last_total_output"`

	// Active model generation window (while agent_state is working/thinking).
	WorkingStartMs int64 `json:"working_start_ms,omitempty"`
	WorkingBaseOut int   `json:"working_base_out,omitempty"`

	// Grace window when agent_state leaves working before total_output_tokens flushes.
	PendingStartMs int64 `json:"pending_start_ms,omitempty"`
	PendingEndMs   int64 `json:"pending_end_ms,omitempty"`
	PendingBaseOut int   `json:"pending_base_out,omitempty"`

	// Most recent finalized step measurement (pure live mode).
	LastTPS        float64 `json:"last_tps,omitempty"`
	LastTokens     int     `json:"last_tokens,omitempty"`
	LastDurationMs int64   `json:"last_duration_ms,omitempty"`

	// Most recent substantial (>=1000 tokens) step measurement (pure live mode).
	LastSubstantialTPS    float64 `json:"last_substantial_tps,omitempty"`
	LastSubstantialTokens int     `json:"last_substantial_tokens,omitempty"`
	LastSubstantialDurMs  int64   `json:"last_substantial_dur_ms,omitempty"`

	// Cumulative finalized steps in this conversation (pure live mode).
	SessionTokens int   `json:"session_tokens,omitempty"`
	SessionDurMs  int64 `json:"session_dur_ms,omitempty"`

	// Finalized text-only PLANNER_RESPONSE measurements (hybrid transcript+live mode).
	SeenTextStepIndex int             `json:"seen_text_step_index,omitempty"`
	LastTextStepIndex int             `json:"last_text_step_index,omitempty"`
	LastTextTPS       float64         `json:"last_text_tps,omitempty"`
	LastTextTokens    int             `json:"last_text_tokens,omitempty"`
	LastTextDurMs     int64           `json:"last_text_dur_ms,omitempty"`
	TextSessionTokens int             `json:"text_session_tokens,omitempty"`
	TextSessionDurMs  int64           `json:"text_session_dur_ms,omitempty"`
	TextSteps         []tpsStepRecord `json:"text_steps,omitempty"`

	// Recent working->idle transition timestamp so a text-only PLANNER_RESPONSE
	// flushed to transcript.jsonl slightly after going idle keeps exact end time.
	LastIdleAtMs           int64 `json:"last_idle_at_ms,omitempty"`
	LastIdleWorkingStartMs int64 `json:"last_idle_working_start_ms,omitempty"`
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

	td := ctx.LoadTranscript()
	hasTranscriptSteps := td != nil && (td.LastTPS > 0 || td.LatestTextStep.StepIndex > 0)

	if sessionID := resolveSessionID(ctx.Stdin); isSafeSessionID(sessionID) {
		if hasTranscriptSteps {
			m.collectHybrid(ctx.Stdin, td, sessionID)
			return nil
		}
		m.collectLive(ctx.Stdin, sessionID)
	}

	// Fall back to transcript-derived TPS when live state has no measurement yet
	// (e.g. cold start, resumed session, or stateless invocation).
	if m.tps <= 0 && td != nil && td.LastTPS > 0 {
		steps := collectTranscriptSteps(td)
		if chosen, ok := selectDisplayStep(steps); ok {
			m.tps = computeTPS(chosen.Tokens, chosen.DurMs)
			m.tokens = chosen.Tokens
			m.duration = time.Duration(chosen.DurMs) * time.Millisecond
		} else {
			m.tps = td.LastTPS
			m.tokens = td.LastOutputTokens
			m.duration = td.LastDuration
		}
		m.avg = td.AvgTPS
		if dec, ttft, ok := estimateDecodeAndTTFT(steps); ok {
			m.decode = dec
			m.ttft = ttft
			m.hasTTFT = true
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

	decodeRateStr := ""
	decodeStr := ""
	if m.decode > 0 && !math.IsNaN(m.decode) && !math.IsInf(m.decode, 0) {
		decodeRateStr = formatRate(m.decode)
		decodeStr = ansi.Colored(ansi.Cyan, decodeRateStr+" tok/s")
	}

	ttftStr := ""
	if m.hasTTFT {
		ttftStr = formatStepDuration(m.ttft)
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
		"tps":         ansi.Colored(ansi.Cyan, rateStr+" tok/s"),
		"rate":        rateStr,
		"avg":         avgStr,
		"avg_rate":    avgRateStr,
		"decode":      decodeStr,
		"decode_rate": decodeRateStr,
		"ttft":        ttftStr,
		"tokens":      tokensStr,
		"duration":    durStr,
	}
}

func (m *TPSModule) DefaultFormat() string {
	return "{tps}"
}

// collectHybrid combines exact per-step transcript boundaries (which exclude tool
// execution time across multi-step agent loops) with live completion timestamps
// for text-only final replies (which have no following GENERIC step in transcript).
func (m *TPSModule) collectHybrid(in *stdin.Data, td *transcript.Data, sessionID string) {
	nowFn := m.now
	if nowFn == nil {
		nowFn = time.Now
	}
	nowMs := nowFn().UnixMilli()

	path := m.stateFilePath(sessionID)
	st, existed := loadTPSState(path)
	prev := st

	if curModel := strings.TrimSpace(in.ModelName()); curModel != "" {
		if st.ActiveModel != "" && st.ActiveModel != curModel {
			st.resetForModelSwitch()
		}
		st.ActiveModel = curModel
	}

	totalOut := max(0, in.ContextWindow.TotalOutputTokens)
	isWorking := isWorkingState(in.AgentState) && !in.ToolConfirmation

	// Expire stale working / idle grace timestamps.
	if st.WorkingStartMs > 0 {
		elapsed := nowMs - st.WorkingStartMs
		if elapsed < 0 || elapsed > maxTPSDuration.Milliseconds() {
			st.clearWorking()
		}
	}
	if st.LastIdleAtMs > 0 {
		sinceIdle := nowMs - st.LastIdleAtMs
		if sinceIdle < 0 || sinceIdle > pendingGraceWindow.Milliseconds() {
			st.LastIdleAtMs = 0
			st.LastIdleWorkingStartMs = 0
		}
	}

	wasWorkingStart := st.WorkingStartMs
	wasIdleAt := st.LastIdleAtMs
	wasIdleWorkStart := st.LastIdleWorkingStartMs
	hadLiveActivity := existed && (wasWorkingStart > 0 || wasIdleAt > 0 || totalOut > st.LastTotalOutput)

	if isWorking {
		if st.WorkingStartMs == 0 {
			st.WorkingStartMs = nowMs
			st.WorkingBaseOut = totalOut
		}
		st.LastIdleAtMs = 0
		st.LastIdleWorkingStartMs = 0
	} else {
		if st.WorkingStartMs > 0 {
			st.LastIdleAtMs = nowMs
			st.LastIdleWorkingStartMs = st.WorkingStartMs
			st.clearWorking()
		}
	}

	// If a new text-only PLANNER_RESPONSE completed while we were actively observing
	// this session, measure it from its own CreatedAt (or sub-second WorkingStartMs
	// on single-step replies) to completion time. Always advance SeenTextStepIndex
	// so stale historical text steps seen on cold start are never measured later.
	ts := td.LatestTextStep
	if ts.StepIndex > 0 && ts.StepIndex > st.SeenTextStepIndex {
		if ts.StepIndex > td.LastStepIndex && ts.StepIndex > st.LastTextStepIndex && ts.OutputTokens > 0 && !ts.CreatedAt.IsZero() && hadLiveActivity {
			endMs := nowMs
			workStart := wasWorkingStart
			if !isWorking && wasWorkingStart == 0 && wasIdleAt > 0 {
				endMs = wasIdleAt
				workStart = wasIdleWorkStart
			}
			startMs := ts.CreatedAt.UnixMilli()
			// Preserve sub-second precision only when CreatedAt is whole-second
			// truncated and WorkingStartMs lands within the same second (<1000ms).
			if ts.CreatedAt.Nanosecond() == 0 && workStart > startMs && workStart-startMs < 1000 && endMs-workStart >= minTPSDuration.Milliseconds() {
				startMs = workStart
			}
			durMs := endMs - startMs
			if isValidDuration(durMs) {
				st.LastTextStepIndex = ts.StepIndex
				st.LastTextTPS = computeTPS(ts.OutputTokens, durMs)
				st.LastTextTokens = ts.OutputTokens
				st.LastTextDurMs = durMs
				st.TextSessionTokens += ts.OutputTokens
				st.TextSessionDurMs += durMs
				st.appendTextStep(tpsStepRecord{
					StepIndex: ts.StepIndex,
					Tokens:    ts.OutputTokens,
					DurMs:     durMs,
				})
			}
		}
		st.SeenTextStepIndex = ts.StepIndex
	}

	st.LastTotalOutput = totalOut
	st.LastSeenMs = nowMs

	if tpsStateChanged(prev, st) || (!existed && (isWorking || totalOut > 0)) {
		if err := saveTPSState(path, st); err != nil {
			debug.Log("tps", "save state %s: %v", path, err)
		} else if !existed {
			pruneStaleTPSState(filepath.Dir(path), nowFn())
		}
	}

	// Combine finalized tool-calling steps and live-timed text-only replies.
	allSteps := collectTranscriptSteps(td)
	if len(st.TextSteps) > 0 {
		allSteps = append(allSteps, st.TextSteps...)
	} else if ts.StepIndex > td.LastStepIndex && st.LastTextStepIndex == ts.StepIndex && st.LastTextTPS > 0 {
		allSteps = append(allSteps, tpsStepRecord{
			StepIndex: st.LastTextStepIndex,
			Tokens:    st.LastTextTokens,
			DurMs:     st.LastTextDurMs,
		})
	}

	if chosen, ok := selectDisplayStep(allSteps); ok {
		m.tps = computeTPS(chosen.Tokens, chosen.DurMs)
		m.tokens = chosen.Tokens
		m.duration = time.Duration(chosen.DurMs) * time.Millisecond
	} else if td.LastTPS > 0 {
		m.tps = td.LastTPS
		m.tokens = td.LastOutputTokens
		m.duration = td.LastDuration
	} else if st.LastTextTPS > 0 {
		m.tps = st.LastTextTPS
		m.tokens = st.LastTextTokens
		m.duration = time.Duration(st.LastTextDurMs) * time.Millisecond
	}

	totTokens := td.TotalOutputTokens + st.TextSessionTokens
	totDurMs := td.TotalDuration.Milliseconds() + st.TextSessionDurMs
	if totTokens > 0 && totDurMs > 0 {
		m.avg = computeTPS(totTokens, totDurMs)
	} else if td.AvgTPS > 0 {
		m.avg = td.AvgTPS
	}

	if dec, ttft, ok := estimateDecodeAndTTFT(allSteps); ok {
		m.decode = dec
		m.ttft = ttft
		m.hasTTFT = true
	}
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

	if curModel := strings.TrimSpace(in.ModelName()); curModel != "" {
		if st.ActiveModel != "" && st.ActiveModel != curModel {
			st.resetForModelSwitch()
		}
		st.ActiveModel = curModel
	}

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
			st.LastIdleAtMs = nowMs
			st.LastIdleWorkingStartMs = st.WorkingStartMs
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

	if liveTPS > 0 && (st.LastSubstantialTPS <= 0 || liveTokens >= minSubstantialTokens) {
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

	if st.LastSubstantialTPS > 0 {
		m.tps = st.LastSubstantialTPS
		m.tokens = st.LastSubstantialTokens
		m.duration = time.Duration(st.LastSubstantialDurMs) * time.Millisecond
		if st.SessionTokens > 0 && st.SessionDurMs > 0 {
			m.avg = computeTPS(st.SessionTokens, st.SessionDurMs)
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

func (s *tpsState) resetForModelSwitch() {
	s.clearWorking()
	s.clearPending()
	s.LastTPS = 0
	s.LastTokens = 0
	s.LastDurationMs = 0
	s.LastSubstantialTPS = 0
	s.LastSubstantialTokens = 0
	s.LastSubstantialDurMs = 0
	s.SessionTokens = 0
	s.SessionDurMs = 0
	s.LastTextStepIndex = 0
	s.LastTextTPS = 0
	s.LastTextTokens = 0
	s.LastTextDurMs = 0
	s.TextSessionTokens = 0
	s.TextSessionDurMs = 0
	s.TextSteps = nil
	s.LastIdleAtMs = 0
	s.LastIdleWorkingStartMs = 0
}

func (s *tpsState) appendTextStep(rec tpsStepRecord) {
	s.TextSteps = append(s.TextSteps, rec)
	if len(s.TextSteps) > maxSavedTextSteps {
		s.TextSteps = s.TextSteps[len(s.TextSteps)-maxSavedTextSteps:]
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
	rate := computeTPS(delta, durMs)
	s.LastTPS = rate
	s.LastTokens = delta
	s.LastDurationMs = durMs
	if delta >= minSubstantialTokens {
		s.LastSubstantialTPS = rate
		s.LastSubstantialTokens = delta
		s.LastSubstantialDurMs = durMs
	}
	s.SessionTokens += delta
	s.SessionDurMs += durMs
}

func collectTranscriptSteps(td *transcript.Data) []tpsStepRecord {
	if td == nil {
		return nil
	}
	if len(td.TPSSteps) > 0 {
		out := make([]tpsStepRecord, 0, len(td.TPSSteps))
		for _, s := range td.TPSSteps {
			if s.OutputTokens > 0 && s.Duration > 0 {
				out = append(out, tpsStepRecord{
					StepIndex: s.StepIndex,
					Tokens:    s.OutputTokens,
					DurMs:     s.Duration.Milliseconds(),
				})
			}
		}
		return out
	}
	if td.LastOutputTokens > 0 && td.LastDuration > 0 {
		return []tpsStepRecord{{
			StepIndex: td.LastStepIndex,
			Tokens:    td.LastOutputTokens,
			DurMs:     td.LastDuration.Milliseconds(),
		}}
	}
	return nil
}

// selectDisplayStep returns the latest substantial (>= minSubstantialTokens)
// step so short TTFT-dominated replies do not tank primary {tps}; when no
// substantial step exists yet, it falls back to the latest step overall.
func selectDisplayStep(steps []tpsStepRecord) (tpsStepRecord, bool) {
	var bestSubstantial tpsStepRecord
	hasSubstantial := false
	var bestAny tpsStepRecord
	hasAny := false

	for _, s := range steps {
		if s.Tokens <= 0 || s.DurMs <= 0 {
			continue
		}
		if !hasAny || s.StepIndex >= bestAny.StepIndex {
			bestAny = s
			hasAny = true
		}
		if s.Tokens >= minSubstantialTokens && (!hasSubstantial || s.StepIndex >= bestSubstantial.StepIndex) {
			bestSubstantial = s
			hasSubstantial = true
		}
	}
	if hasSubstantial {
		return bestSubstantial, true
	}
	return bestAny, hasAny
}

// estimateDecodeAndTTFT fits duration = TTFT + tokens / decode_tps via ordinary
// least squares over finalized steps. Returns ok=false unless sample count,
// token spread, goodness-of-fit (R^2), and physical bounds all pass.
func estimateDecodeAndTTFT(steps []tpsStepRecord) (float64, time.Duration, bool) {
	var valid []tpsStepRecord
	minTok, maxTok := math.MaxInt, 0
	for _, s := range steps {
		if s.Tokens <= 0 || s.DurMs <= 0 {
			continue
		}
		valid = append(valid, s)
		if s.Tokens < minTok {
			minTok = s.Tokens
		}
		if s.Tokens > maxTok {
			maxTok = s.Tokens
		}
	}
	if len(valid) < minOLSSamples || maxTok-minTok < minOLSTokenSpread {
		return 0, 0, false
	}

	n := float64(len(valid))
	var sumX, sumY float64
	for _, s := range valid {
		sumX += float64(s.Tokens)
		sumY += float64(s.DurMs) / 1000.0
	}
	meanX := sumX / n
	meanY := sumY / n

	var sxx, syy, sxy float64
	for _, s := range valid {
		dx := float64(s.Tokens) - meanX
		dy := float64(s.DurMs)/1000.0 - meanY
		sxx += dx * dx
		syy += dy * dy
		sxy += dx * dy
	}
	if sxx <= 0 || syy <= 0 || sxy <= 0 {
		return 0, 0, false
	}
	r2 := (sxy * sxy) / (sxx * syy)
	if r2 < minOLSR2 {
		return 0, 0, false
	}

	slope := sxy / sxx
	if slope <= 0 {
		return 0, 0, false
	}
	decodeTPS := 1.0 / slope
	ttftSec := meanY - slope*meanX
	if math.IsNaN(decodeTPS) || math.IsInf(decodeTPS, 0) || decodeTPS < 1.0 || decodeTPS > maxOLSDecodeTPS {
		return 0, 0, false
	}
	if math.IsNaN(ttftSec) || math.IsInf(ttftSec, 0) || ttftSec < 0 || ttftSec > maxOLSTTFTSec {
		return 0, 0, false
	}
	return decodeTPS, time.Duration(math.Round(ttftSec * float64(time.Second))), true
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
	return a.ActiveModel != b.ActiveModel ||
		a.LastTotalOutput != b.LastTotalOutput ||
		a.WorkingStartMs != b.WorkingStartMs ||
		a.WorkingBaseOut != b.WorkingBaseOut ||
		a.PendingStartMs != b.PendingStartMs ||
		a.PendingEndMs != b.PendingEndMs ||
		a.PendingBaseOut != b.PendingBaseOut ||
		a.LastTPS != b.LastTPS ||
		a.LastTokens != b.LastTokens ||
		a.LastDurationMs != b.LastDurationMs ||
		a.LastSubstantialTPS != b.LastSubstantialTPS ||
		a.LastSubstantialTokens != b.LastSubstantialTokens ||
		a.LastSubstantialDurMs != b.LastSubstantialDurMs ||
		a.SessionTokens != b.SessionTokens ||
		a.SessionDurMs != b.SessionDurMs ||
		a.SeenTextStepIndex != b.SeenTextStepIndex ||
		a.LastTextStepIndex != b.LastTextStepIndex ||
		a.LastTextTPS != b.LastTextTPS ||
		a.LastTextTokens != b.LastTextTokens ||
		a.LastTextDurMs != b.LastTextDurMs ||
		a.TextSessionTokens != b.TextSessionTokens ||
		a.TextSessionDurMs != b.TextSessionDurMs ||
		!slices.Equal(a.TextSteps, b.TextSteps) ||
		a.LastIdleAtMs != b.LastIdleAtMs ||
		a.LastIdleWorkingStartMs != b.LastIdleWorkingStartMs
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
