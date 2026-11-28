package module

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mritd/agy-statusline/internal/ansi"
	"github.com/mritd/agy-statusline/internal/config"
	"github.com/mritd/agy-statusline/internal/debug"
	"github.com/mritd/agy-statusline/internal/transcript"
)

const (
	maxGoalTextLen   = 40
	goalCacheMaxAge  = 7 * 24 * time.Hour
	maxGoalCacheSize = 4096
)

// GoalModule displays active /goal mode status and elapsed runtime.
// It reads /goal lifecycle markers from transcript.jsonl and keeps a small
// per-session cache only while a goal is active so long-running goals whose
// start scrolled past max_tail_size remain visible until completed or cancelled.
type GoalModule struct {
	stateDir string
	now      func() time.Time

	active    bool
	startTime time.Time
	text      string
}

type goalCache struct {
	StartMs int64  `json:"start_ms"`
	Text    string `json:"text,omitempty"`
}

func NewGoalModule() *GoalModule {
	return &GoalModule{now: time.Now}
}

func (m *GoalModule) Name() string { return "goal" }

func (m *GoalModule) Collect(ctx *Context) error {
	m.active = false
	m.startTime = time.Time{}
	m.text = ""

	td := ctx.LoadTranscript()
	if td == nil {
		return nil
	}

	sessionID := resolveSessionID(ctx.Stdin)
	hasSafeID := isSafeSessionID(sessionID)

	switch td.Goal.Status {
	case transcript.GoalActive:
		m.active = true
		m.startTime = td.Goal.StartTime
		m.text = sanitizeSingleLine(td.Goal.Text, maxGoalTextLen)
		if hasSafeID {
			m.saveActiveCache(sessionID)
		}

	case transcript.GoalCompleted, transcript.GoalCancelled:
		if hasSafeID {
			m.removeCache(sessionID)
		}

	case transcript.GoalNone:
		if !hasSafeID {
			return nil
		}
		if td.FullScan {
			// Full transcript scanned cleanly with no /goal; remove any stale cache.
			m.removeCache(sessionID)
			return nil
		}
		// Partial scan (file exceeded max_tail_size or open/scan failed):
		// preserve and restore an active goal cached earlier in this session.
		if cached, ok := m.loadCache(sessionID); ok {
			m.active = true
			if cached.StartMs > 0 {
				m.startTime = time.UnixMilli(cached.StartMs)
			}
			m.text = sanitizeSingleLine(cached.Text, maxGoalTextLen)
		}
	}

	return nil
}

func (m *GoalModule) Vars(ctx *Context) map[string]string {
	if !m.active {
		return nil
	}

	elapsedStr := ""
	if !m.startTime.IsZero() {
		elapsedStr = formatGoalElapsed(m.now().Sub(m.startTime))
	}

	label := "Goal"
	if elapsedStr != "" {
		label = "Goal " + elapsedStr
	}
	if m.text != "" {
		label += " >> " + m.text
	}

	return map[string]string{
		"goal":    ansi.Colored(ansi.BrightMagenta, label),
		"status":  "active",
		"elapsed": elapsedStr,
		"text":    m.text,
	}
}

func (m *GoalModule) DefaultFormat() string {
	return "{goal}"
}

func formatGoalElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

func (m *GoalModule) cacheDir() string {
	if m.stateDir != "" {
		return m.stateDir
	}
	return filepath.Join(config.PluginDir(), "cache", "goal")
}

func (m *GoalModule) cacheFilePath(sessionID string) string {
	return filepath.Join(m.cacheDir(), sessionID+".json")
}

func (m *GoalModule) loadCache(sessionID string) (goalCache, bool) {
	content, ok := readSmallRegularFile(m.cacheFilePath(sessionID), maxGoalCacheSize)
	if !ok {
		return goalCache{}, false
	}
	var c goalCache
	if err := json.Unmarshal([]byte(content), &c); err != nil {
		debug.Log("goal", "corrupt cache %s: %v", sessionID, err)
		return goalCache{}, false
	}
	return c, true
}

func (m *GoalModule) saveActiveCache(sessionID string) {
	desired := goalCache{Text: m.text}
	if !m.startTime.IsZero() {
		desired.StartMs = m.startTime.UnixMilli()
	}

	// Skip redundant writes on subsequent ticks when the same goal is already cached.
	if existing, ok := m.loadCache(sessionID); ok && existing == desired {
		return
	}

	path := m.cacheFilePath(sessionID)
	if err := writeJSONAtomic(path, ".goal-*.tmp", desired); err != nil {
		debug.Log("goal", "save cache %s: %v", path, err)
		return
	}
	pruneStaleJSONFiles(filepath.Dir(path), m.now(), goalCacheMaxAge)
}

func (m *GoalModule) removeCache(sessionID string) {
	_ = os.Remove(m.cacheFilePath(sessionID))
}
