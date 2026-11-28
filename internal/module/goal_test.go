package module

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mritd/agy-statusline/internal/ansi"
	"github.com/mritd/agy-statusline/internal/stdin"
	"github.com/mritd/agy-statusline/internal/transcript"
)

func TestGoalModule_ActiveCompletedCancelledAndCache(t *testing.T) {
	cacheDir := t.TempDir()
	start := time.Date(2026, 11, 28, 13, 0, 0, 0, time.UTC)
	now := start.Add(12*time.Minute + 30*time.Second)

	newMod := func(tNow time.Time) *GoalModule {
		return &GoalModule{
			stateDir: cacheDir,
			now:      func() time.Time { return tNow },
		}
	}

	// 1. Active /goal in transcript -> displays Goal 12m >> ... and writes cache.
	m1 := newMod(now)
	ctxActive := &Context{
		Stdin: &stdin.Data{ConversationID: "conv-goal-1"},
		Transcript: &transcript.Data{
			FullScan: true,
			Goal: transcript.GoalState{
				Status:    transcript.GoalActive,
				StartTime: start,
				Text:      "跑三轮 /review, 对发现的问题全部修复",
			},
		},
	}
	if err := m1.Collect(ctxActive); err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	vars := m1.Vars(ctxActive)
	if vars == nil {
		t.Fatal("expected non-nil vars while goal is active")
	}
	if vars["goal"] != ansi.Colored(ansi.BrightMagenta, "Goal 12m >> 跑三轮 /review, 对发现的问题全部修复") {
		t.Errorf("goal = %q", vars["goal"])
	}
	if vars["status"] != "active" || vars["elapsed"] != "12m" || vars["text"] != "跑三轮 /review, 对发现的问题全部修复" {
		t.Errorf("vars = %+v", vars)
	}
	cacheFile := filepath.Join(cacheDir, "conv-goal-1.json")
	if _, err := os.Stat(cacheFile); err != nil {
		t.Fatalf("expected active goal cache file: %v", err)
	}

	// Subsequent identical active tick skips redundant cache rewrite.
	pastMtime := start.Add(-1 * time.Hour)
	if err := os.Chtimes(cacheFile, pastMtime, pastMtime); err != nil {
		t.Fatal(err)
	}
	_ = newMod(now).Collect(ctxActive)
	info, err := os.Stat(cacheFile)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(pastMtime) {
		t.Errorf("expected identical active goal tick to skip rewrite, mtime changed from %v to %v", pastMtime, info.ModTime())
	}

	// Active goal without text displays Goal <elapsed> without separator.
	mNoText := newMod(now)
	ctxNoText := &Context{
		Transcript: &transcript.Data{
			FullScan: true,
			Goal:     transcript.GoalState{Status: transcript.GoalActive, StartTime: start},
		},
	}
	_ = mNoText.Collect(ctxNoText)
	if varsNoText := mNoText.Vars(ctxNoText); varsNoText == nil || varsNoText["goal"] != ansi.Colored(ansi.BrightMagenta, "Goal 12m") {
		t.Errorf("vars without text = %+v, want colored Goal 12m", varsNoText)
	}

	// 2. Partial scan (FullScan=false, GoalNone: e.g. tail-seeked or transient read failure) -> restored from cache.
	m2 := newMod(start.Add(2*time.Hour + 5*time.Minute))
	ctxPartial := &Context{
		Stdin: &stdin.Data{ConversationID: "conv-goal-1"},
		Transcript: &transcript.Data{
			FullScan: false,
			Goal:     transcript.GoalState{Status: transcript.GoalNone},
		},
	}
	_ = m2.Collect(ctxPartial)
	vars2 := m2.Vars(ctxPartial)
	if vars2 == nil || vars2["elapsed"] != "2h 5m" || vars2["goal"] != ansi.Colored(ansi.BrightMagenta, "Goal 2h 5m >> 跑三轮 /review, 对发现的问题全部修复") {
		t.Fatalf("cached active goal vars = %+v", vars2)
	}

	// 3. Goal cancelled -> hides and removes cache.
	m3 := newMod(now)
	ctxCancelled := &Context{
		Stdin: &stdin.Data{ConversationID: "conv-goal-1"},
		Transcript: &transcript.Data{
			FullScan: false,
			Goal:     transcript.GoalState{Status: transcript.GoalCancelled},
		},
	}
	_ = m3.Collect(ctxCancelled)
	if vars3 := m3.Vars(ctxCancelled); vars3 != nil {
		t.Errorf("expected nil vars when goal cancelled, got %+v", vars3)
	}
	if _, err := os.Stat(cacheFile); !os.IsNotExist(err) {
		t.Errorf("expected cache file removed on cancel, stat err = %v", err)
	}

	// 4. Re-cache then complete -> hides and removes cache.
	_ = newMod(now).Collect(ctxActive)
	m5 := newMod(now)
	ctxCompleted := &Context{
		Stdin: &stdin.Data{ConversationID: "conv-goal-1"},
		Transcript: &transcript.Data{
			FullScan: true,
			Goal:     transcript.GoalState{Status: transcript.GoalCompleted},
		},
	}
	_ = m5.Collect(ctxCompleted)
	if vars5 := m5.Vars(ctxCompleted); vars5 != nil {
		t.Errorf("expected nil vars when goal completed, got %+v", vars5)
	}
	if _, err := os.Stat(cacheFile); !os.IsNotExist(err) {
		t.Errorf("expected cache file removed on complete, stat err = %v", err)
	}
}

func TestGoalModule_CachePruneAndEdgeCases(t *testing.T) {
	cacheDir := t.TempDir()
	now := time.Date(2026, 11, 28, 15, 0, 0, 0, time.UTC)

	// Full scan with no goal removes stale cache for this session.
	stale := filepath.Join(cacheDir, "conv-stale.json")
	if err := os.WriteFile(stale, []byte(`{"start_ms":1000}`), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &GoalModule{stateDir: cacheDir, now: func() time.Time { return now }}
	ctx := &Context{
		Stdin:      &stdin.Data{ConversationID: "conv-stale"},
		Transcript: &transcript.Data{FullScan: true, Goal: transcript.GoalState{Status: transcript.GoalNone}},
	}
	_ = m.Collect(ctx)
	if vars := m.Vars(ctx); vars != nil {
		t.Errorf("expected nil vars on non-goal full scan, got %+v", vars)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("expected stale cache removed on full scan with no goal, err = %v", err)
	}

	// Saving a new active goal prunes >7d old cache files while keeping fresh ones.
	oldFile := filepath.Join(cacheDir, "conv-old.json")
	freshFile := filepath.Join(cacheDir, "conv-fresh.json")
	if err := os.WriteFile(oldFile, []byte(`{"start_ms":1000}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(freshFile, []byte(`{"start_ms":2000}`), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := now.Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(freshFile, now, now); err != nil {
		t.Fatal(err)
	}
	mNew := &GoalModule{stateDir: cacheDir, now: func() time.Time { return now }}
	_ = mNew.Collect(&Context{
		Stdin: &stdin.Data{ConversationID: "conv-new"},
		Transcript: &transcript.Data{
			FullScan: true,
			Goal:     transcript.GoalState{Status: transcript.GoalActive, StartTime: now},
		},
	})
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Errorf("expected >7d old cache file pruned, stat err = %v", err)
	}
	if _, err := os.Stat(freshFile); err != nil {
		t.Errorf("expected fresh cache file preserved: %v", err)
	}

	// Corrupt cache, unsafe session ID, zero StartTime, and >40-rune CJK truncation.
	corruptFile := filepath.Join(cacheDir, "conv-corrupt.json")
	if err := os.WriteFile(corruptFile, []byte("{bad json"), 0o600); err != nil {
		t.Fatal(err)
	}
	mCorrupt := &GoalModule{stateDir: cacheDir, now: func() time.Time { return now }}
	ctxCorrupt := &Context{
		Stdin:      &stdin.Data{ConversationID: "conv-corrupt"},
		Transcript: &transcript.Data{FullScan: false, Goal: transcript.GoalState{Status: transcript.GoalNone}},
	}
	_ = mCorrupt.Collect(ctxCorrupt)
	if vars := mCorrupt.Vars(ctxCorrupt); vars != nil {
		t.Errorf("expected nil vars on corrupt cache, got %+v", vars)
	}

	// Unsafe session ID must not write any file.
	mUnsafe := &GoalModule{stateDir: cacheDir, now: func() time.Time { return now }}
	longText := strings.Repeat("测", 45)
	ctxUnsafe := &Context{
		Stdin: &stdin.Data{ConversationID: "../escape"},
		Transcript: &transcript.Data{
			FullScan: true,
			Goal:     transcript.GoalState{Status: transcript.GoalActive, Text: longText},
		},
	}
	_ = mUnsafe.Collect(ctxUnsafe)
	varsUnsafe := mUnsafe.Vars(ctxUnsafe)
	if varsUnsafe == nil || varsUnsafe["elapsed"] != "" || !strings.HasSuffix(varsUnsafe["text"], "...") || len([]rune(varsUnsafe["text"])) != maxGoalTextLen+3 {
		t.Errorf("zero-start/truncated vars = %+v", varsUnsafe)
	}
	if varsUnsafe["goal"] != ansi.Colored(ansi.BrightMagenta, "Goal >> "+varsUnsafe["text"]) {
		t.Errorf("zero-start goal label = %q", varsUnsafe["goal"])
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "escape") {
			t.Errorf("unexpected file created for unsafe session ID: %s", e.Name())
		}
	}

	tests := []struct {
		dur  time.Duration
		want string
	}{
		{-5 * time.Second, "0s"},
		{45 * time.Second, "45s"},
		{5 * time.Minute, "5m"},
		{90 * time.Minute, "1h 30m"},
	}
	for _, tt := range tests {
		if got := formatGoalElapsed(tt.dur); got != tt.want {
			t.Errorf("formatGoalElapsed(%v) = %q, want %q", tt.dur, got, tt.want)
		}
	}
}
