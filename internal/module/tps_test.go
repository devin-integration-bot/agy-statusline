package module

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mritd/agy-statusline/internal/ansi"
	"github.com/mritd/agy-statusline/internal/stdin"
	"github.com/mritd/agy-statusline/internal/transcript"
)

func newTestTPSModule(stateDir string, now func() time.Time) *TPSModule {
	return &TPSModule{
		stateDir: stateDir,
		now:      now,
	}
}

func TestTPSModule_LiveWorkingToToolUseAndIdle(t *testing.T) {
	stateDir := t.TempDir()
	base := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)

	runTick := func(offset time.Duration, state string, totalOut int, confirming bool) map[string]string {
		m := newTestTPSModule(stateDir, func() time.Time { return base.Add(offset) })
		ctx := &Context{
			Stdin: &stdin.Data{
				ConversationID:   "conv-live-1",
				AgentState:       state,
				ToolConfirmation: confirming,
				ContextWindow: stdin.ContextWindow{
					TotalOutputTokens: totalOut,
				},
			},
		}
		if err := m.Collect(ctx); err != nil {
			t.Fatalf("Collect error: %v", err)
		}
		return m.Vars(ctx)
	}

	// Step 1 starts: no measurement yet.
	if vars := runTick(0, "working", 0, false); vars != nil {
		t.Fatalf("expected nil vars on initial working tick, got %v", vars)
	}

	// Step 1 finishes after 5s with 500 tokens -> 100.0 tok/s.
	vars := runTick(5*time.Second, "tool_use", 500, false)
	if vars == nil {
		t.Fatal("expected vars after step 1 completion")
	}
	if vars["rate"] != "100.0" || vars["avg_rate"] != "100.0" || vars["tokens"] != "500" || vars["duration"] != "5.0s" {
		t.Errorf("step 1 vars = %+v", vars)
	}
	if vars["tps"] != ansi.Colored(ansi.Cyan, "100.0 tok/s") {
		t.Errorf("step 1 tps = %q", vars["tps"])
	}

	// Step 2 (pure text response, working -> idle) after 4s with 200 tokens -> 50.0 tok/s, avg = 700/9 = 77.8 tok/s.
	_ = runTick(10*time.Second, "working", 500, false)
	vars = runTick(14*time.Second, "idle", 700, false)
	if vars == nil {
		t.Fatal("expected vars after step 2 completion")
	}
	if vars["rate"] != "50.0" || vars["avg_rate"] != "77.8" || vars["tokens"] != "200" || vars["duration"] != "4.0s" {
		t.Errorf("step 2 vars = %+v", vars)
	}
}

func TestTPSModule_DelayedTokenFlushGraceWindow(t *testing.T) {
	stateDir := t.TempDir()
	base := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)

	tick := func(offset time.Duration, state string, totalOut int) map[string]string {
		m := newTestTPSModule(stateDir, func() time.Time { return base.Add(offset) })
		ctx := &Context{
			Stdin: &stdin.Data{
				ConversationID: "conv-grace-1",
				AgentState:     state,
				ContextWindow:  stdin.ContextWindow{TotalOutputTokens: totalOut},
			},
		}
		_ = m.Collect(ctx)
		return m.Vars(ctx)
	}

	// Working starts at t=0 with 100 tokens.
	_ = tick(0, "working", 100)
	// Exits working at t=4s, but token count hasn't flushed yet (still 100).
	if vars := tick(4*time.Second, "idle", 100); vars != nil {
		t.Fatalf("expected nil before tokens flush, got %+v", vars)
	}
	// Tokens arrive 300ms later while idle -> duration should be the 4s working window -> 100.0 tok/s.
	vars := tick(4300*time.Millisecond, "idle", 500)
	if vars == nil {
		t.Fatal("expected vars after delayed flush")
	}
	if vars["rate"] != "100.0" || vars["duration"] != "4.0s" || vars["tokens"] != "400" {
		t.Errorf("delayed flush vars = %+v", vars)
	}
}

func TestTPSModule_StreamingLiveNoDoubleCount(t *testing.T) {
	stateDir := t.TempDir()
	base := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)

	tick := func(offset time.Duration, state string, totalOut int) map[string]string {
		m := newTestTPSModule(stateDir, func() time.Time { return base.Add(offset) })
		ctx := &Context{
			Stdin: &stdin.Data{
				ConversationID: "conv-stream-1",
				AgentState:     state,
				ContextWindow:  stdin.ContextWindow{TotalOutputTokens: totalOut},
			},
		}
		_ = m.Collect(ctx)
		return m.Vars(ctx)
	}

	_ = tick(0, "working", 0)

	// Mid-stream update at 2s: 160 tokens -> 80.0 tok/s live.
	vars := tick(2*time.Second, "working", 160)
	if vars == nil || vars["rate"] != "80.0" || vars["avg_rate"] != "80.0" {
		t.Fatalf("mid-stream 1 vars = %+v", vars)
	}

	// Second mid-stream update at 4s: 400 tokens -> 100.0 tok/s live.
	vars = tick(4*time.Second, "working", 400)
	if vars == nil || vars["rate"] != "100.0" || vars["avg_rate"] != "100.0" {
		t.Fatalf("mid-stream 2 vars = %+v", vars)
	}

	// Finalize at 5s: 500 tokens -> 100.0 tok/s, session totals should be 500 tok / 5s (not double-counted).
	vars = tick(5*time.Second, "idle", 500)
	if vars == nil || vars["rate"] != "100.0" || vars["avg_rate"] != "100.0" {
		t.Fatalf("finalized vars = %+v", vars)
	}

	// Next step: 5s, 250 tokens -> 50.0 tok/s; avg must be (500+250)/(5+5) = 75.0 tok/s.
	_ = tick(6*time.Second, "working", 500)
	vars = tick(11*time.Second, "idle", 750)
	if vars == nil || vars["rate"] != "50.0" || vars["avg_rate"] != "75.0" {
		t.Fatalf("step 2 after streaming vars = %+v, want rate=50.0 avg_rate=75.0", vars)
	}
}

func TestTPSModule_TranscriptFallbackAndPerSessionIsolation(t *testing.T) {
	stateDir := t.TempDir()
	base := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)

	// Session A gets live state.
	mA1 := newTestTPSModule(stateDir, func() time.Time { return base })
	ctxA := &Context{
		Stdin: &stdin.Data{
			ConversationID: "session-a",
			AgentState:     "working",
			ContextWindow:  stdin.ContextWindow{TotalOutputTokens: 0},
		},
	}
	_ = mA1.Collect(ctxA)

	mA2 := newTestTPSModule(stateDir, func() time.Time { return base.Add(2 * time.Second) })
	ctxA2 := &Context{
		Stdin: &stdin.Data{
			ConversationID: "session-a",
			AgentState:     "idle",
			ContextWindow:  stdin.ContextWindow{TotalOutputTokens: 300},
		},
	}
	_ = mA2.Collect(ctxA2)
	if varsA := mA2.Vars(ctxA2); varsA == nil || varsA["rate"] != "150.0" {
		t.Fatalf("session-a vars = %+v, want rate=150.0", varsA)
	}

	// Session B has no live state yet, should NOT inherit session A's state and should use transcript fallback.
	mB := newTestTPSModule(stateDir, func() time.Time { return base.Add(3 * time.Second) })
	ctxB := &Context{
		Stdin: &stdin.Data{
			ConversationID: "session-b",
			AgentState:     "idle",
		},
		Transcript: &transcript.Data{
			LastTPS:          64.5,
			AvgTPS:           58.2,
			LastOutputTokens: 1290,
			LastDuration:     20 * time.Second,
		},
	}
	_ = mB.Collect(ctxB)
	varsB := mB.Vars(ctxB)
	if varsB == nil {
		t.Fatal("expected transcript fallback vars for session-b")
	}
	if varsB["rate"] != "64.5" || varsB["avg_rate"] != "58.2" || varsB["tokens"] != "1.3k" || varsB["duration"] != "20.0s" {
		t.Errorf("session-b fallback vars = %+v", varsB)
	}

	// Verify separate state file exists for session-a.
	if _, err := os.Stat(filepath.Join(stateDir, "session-a.json")); err != nil {
		t.Errorf("expected session-a.json state file: %v", err)
	}
}

func TestTPSModule_StaleTimerAndUnsafeSessionID(t *testing.T) {
	stateDir := t.TempDir()
	base := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)

	// Start working, then wake up 15 minutes later -> stale timer discarded.
	m1 := newTestTPSModule(stateDir, func() time.Time { return base })
	_ = m1.Collect(&Context{
		Stdin: &stdin.Data{
			ConversationID: "conv-stale",
			AgentState:     "working",
			ContextWindow:  stdin.ContextWindow{TotalOutputTokens: 100},
		},
	})
	m2 := newTestTPSModule(stateDir, func() time.Time { return base.Add(15 * time.Minute) })
	ctx2 := &Context{
		Stdin: &stdin.Data{
			ConversationID: "conv-stale",
			AgentState:     "idle",
			ContextWindow:  stdin.ContextWindow{TotalOutputTokens: 500},
		},
	}
	_ = m2.Collect(ctx2)
	if vars := m2.Vars(ctx2); vars != nil {
		t.Errorf("expected stale timer to be discarded, got %+v", vars)
	}

	// Unsafe session ID must not write any file outside stateDir.
	mUnsafe := newTestTPSModule(stateDir, func() time.Time { return base })
	_ = mUnsafe.Collect(&Context{
		Stdin: &stdin.Data{
			ConversationID: "../escape",
			AgentState:     "working",
		},
	})
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "escape") {
			t.Errorf("unexpected file created for unsafe session ID: %s", e.Name())
		}
	}
}

func TestTPSModule_FormatHelpers(t *testing.T) {
	tests := []struct {
		dur  time.Duration
		want string
	}{
		{50 * time.Millisecond, "<0.1s"},
		{2500 * time.Millisecond, "2.5s"},
		{59960 * time.Millisecond, "1m 0s"},
		{119600 * time.Millisecond, "2m 0s"},
		{125 * time.Second, "2m 5s"},
	}
	for _, tt := range tests {
		if got := formatStepDuration(tt.dur); got != tt.want {
			t.Errorf("formatStepDuration(%v) = %q, want %q", tt.dur, got, tt.want)
		}
	}

	if got := formatStepTokens(850); got != "850" {
		t.Errorf("formatStepTokens(850) = %q", got)
	}
	if got := formatStepTokens(1527); got != "1.5k" {
		t.Errorf("formatStepTokens(1527) = %q", got)
	}
	if got := formatStepTokens(1_250_000); got != "1.2M" && got != "1.3M" {
		t.Errorf("formatStepTokens(1_250_000) = %q", got)
	}
	if got := math.Abs(computeTPS(500, 2500) - 200.0); got > 1e-9 {
		t.Errorf("computeTPS(500, 2500) off by %v", got)
	}
}

func TestTPSModule_EdgeCases(t *testing.T) {
	base := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)

	t.Run("tool_confirmation_pauses_working", func(t *testing.T) {
		dir := t.TempDir()
		tick := func(offset time.Duration, state string, totalOut int, confirming bool) map[string]string {
			m := newTestTPSModule(dir, func() time.Time { return base.Add(offset) })
			ctx := &Context{Stdin: &stdin.Data{
				ConversationID:   "conv-confirm",
				AgentState:       state,
				ToolConfirmation: confirming,
				ContextWindow:    stdin.ContextWindow{TotalOutputTokens: totalOut},
			}}
			_ = m.Collect(ctx)
			return m.Vars(ctx)
		}
		_ = tick(0, "working", 0, false)
		// Confirmation dialog appears at 2s; user takes 30s to approve.
		_ = tick(2*time.Second, "working", 0, true)
		// Tokens flush 500ms after confirmation started (within grace window) -> uses 2s, not 32s.
		vars := tick(2500*time.Millisecond, "tool_use", 200, false)
		if vars == nil || vars["rate"] != "100.0" || vars["duration"] != "2.0s" {
			t.Fatalf("confirmation vars = %+v, want rate=100.0 duration=2.0s", vars)
		}
	})

	t.Run("pending_flush_at_next_working_start", func(t *testing.T) {
		dir := t.TempDir()
		tick := func(offset time.Duration, state string, totalOut int) map[string]string {
			m := newTestTPSModule(dir, func() time.Time { return base.Add(offset) })
			ctx := &Context{Stdin: &stdin.Data{
				ConversationID: "conv-pending-next",
				AgentState:     state,
				ContextWindow:  stdin.ContextWindow{TotalOutputTokens: totalOut},
			}}
			_ = m.Collect(ctx)
			return m.Vars(ctx)
		}
		_ = tick(0, "working", 100)
		_ = tick(4*time.Second, "tool_use", 100) // enters grace window
		// Next working step begins 500ms later with flushed tokens from step 1 -> finalizes step 1 (400/4s = 100 tok/s).
		vars := tick(4500*time.Millisecond, "working", 500)
		if vars == nil || vars["rate"] != "100.0" || vars["duration"] != "4.0s" {
			t.Fatalf("pending-to-working vars = %+v", vars)
		}
	})

	t.Run("pending_grace_expires_and_counter_regression", func(t *testing.T) {
		dir := t.TempDir()
		tick := func(offset time.Duration, state string, totalOut int) map[string]string {
			m := newTestTPSModule(dir, func() time.Time { return base.Add(offset) })
			ctx := &Context{Stdin: &stdin.Data{
				ConversationID: "conv-expire",
				AgentState:     state,
				ContextWindow:  stdin.ContextWindow{TotalOutputTokens: totalOut},
			}}
			_ = m.Collect(ctx)
			return m.Vars(ctx)
		}
		_ = tick(0, "working", 100)
		_ = tick(3*time.Second, "idle", 100)
		// Tokens arrive 10s later (past 5s grace window) -> ignored.
		if vars := tick(13*time.Second, "idle", 500); vars != nil {
			t.Fatalf("expected nil after expired grace window, got %+v", vars)
		}
		// Counter regression resets active timer cleanly.
		_ = tick(20*time.Second, "working", 500)
		if vars := tick(22*time.Second, "idle", 100); vars != nil {
			t.Fatalf("expected nil on counter regression, got %+v", vars)
		}
	})

	t.Run("corrupt_state_recovers_and_prunes_old_files", func(t *testing.T) {
		dir := t.TempDir()
		corruptPath := filepath.Join(dir, "conv-corrupt.json")
		if err := os.WriteFile(corruptPath, []byte("{bad json"), 0o600); err != nil {
			t.Fatal(err)
		}
		oldPath := filepath.Join(dir, "old-session.json")
		if err := os.WriteFile(oldPath, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		oldTime := base.Add(-10 * 24 * time.Hour)
		if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
			t.Fatal(err)
		}
		freshPath := filepath.Join(dir, "fresh-session.json")
		if err := os.WriteFile(freshPath, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(freshPath, base, base); err != nil {
			t.Fatal(err)
		}

		// Corrupt file recovers without error and starts fresh.
		m1 := newTestTPSModule(dir, func() time.Time { return base })
		_ = m1.Collect(&Context{Stdin: &stdin.Data{
			ConversationID: "conv-corrupt",
			AgentState:     "working",
			ContextWindow:  stdin.ContextWindow{TotalOutputTokens: 0},
		}})
		m2 := newTestTPSModule(dir, func() time.Time { return base.Add(2 * time.Second) })
		ctx2 := &Context{Stdin: &stdin.Data{
			ConversationID: "conv-corrupt",
			AgentState:     "idle",
			ContextWindow:  stdin.ContextWindow{TotalOutputTokens: 200},
		}}
		_ = m2.Collect(ctx2)
		if vars := m2.Vars(ctx2); vars == nil || vars["rate"] != "100.0" {
			t.Fatalf("recovered corrupt state vars = %+v", vars)
		}

		// Creating a new session file triggers pruning of >7d old files while preserving fresh ones.
		mNew := newTestTPSModule(dir, func() time.Time { return base })
		_ = mNew.Collect(&Context{Stdin: &stdin.Data{
			ConversationID: "conv-new",
			AgentState:     "working",
		}})
		if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
			t.Errorf("expected old-session.json to be pruned, stat err = %v", err)
		}
		if _, err := os.Stat(freshPath); err != nil {
			t.Errorf("expected fresh-session.json to remain: %v", err)
		}
	})
}
