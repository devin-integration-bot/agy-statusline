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

func TestTPSModule_HybridExcludesToolExecutionAndMeasuresFinalTextReply(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)

	run := func(offset time.Duration, state string, totalOut int, td *transcript.Data) map[string]string {
		m := newTestTPSModule(dir, func() time.Time { return base.Add(offset) })
		ctx := &Context{
			Stdin: &stdin.Data{
				ConversationID: "conv-hybrid-1",
				AgentState:     state,
				ContextWindow:  stdin.ContextWindow{TotalOutputTokens: totalOut},
			},
			Transcript: td,
		}
		if err := m.Collect(ctx); err != nil {
			t.Fatalf("Collect error: %v", err)
		}
		return m.Vars(ctx)
	}

	// Turn starts at t=0; step 1 (tool call) finishes at t=10s (1000 tok / 10s = 100 tok/s).
	// A slow tool then runs for 60s while agent_state stays "working".
	tdStep1 := &transcript.Data{
		LastStepIndex:     1,
		LastTPS:           100.0,
		AvgTPS:            100.0,
		LastOutputTokens:  1000,
		LastDuration:      10 * time.Second,
		TotalOutputTokens: 1000,
		TotalDuration:     10 * time.Second,
	}
	vars := run(10*time.Second, "working", 1000, tdStep1)
	if vars == nil || vars["rate"] != "100.0" {
		t.Fatalf("step 1 vars = %+v, want rate=100.0", vars)
	}
	// 50 seconds into slow tool execution, TPS must stay at 100.0 (not decay toward 16 tok/s!).
	vars = run(60*time.Second, "working", 1000, tdStep1)
	if vars == nil || vars["rate"] != "100.0" {
		t.Fatalf("during slow tool execution vars = %+v, want rate=100.0", vars)
	}

	// Final text-only reply starts at t=70s (step 3, 1500 tok) and finishes at t=80s (150 tok/s).
	// Even though WorkingStartMs was way back at t=10s, duration must be 10s (80s - 70s), NOT 70s!
	tdFinalText := &transcript.Data{
		LastStepIndex:     1,
		LastTPS:           100.0,
		AvgTPS:            100.0,
		LastOutputTokens:  1000,
		LastDuration:      10 * time.Second,
		TotalOutputTokens: 1000,
		TotalDuration:     10 * time.Second,
		LatestTextStep: transcript.TextPlannerStep{
			StepIndex:    3,
			OutputTokens: 1500,
			CreatedAt:    base.Add(70 * time.Second),
		},
	}
	vars = run(80*time.Second, "idle", 2500, tdFinalText)
	if vars == nil || vars["rate"] != "150.0" || vars["duration"] != "10.0s" || vars["avg_rate"] != "125.0" {
		t.Fatalf("final text reply vars = %+v, want rate=150.0 duration=10.0s avg_rate=125.0", vars)
	}

	// Subsequent idle refreshes (30s later) must keep the frozen 150.0 tok/s without drifting.
	vars = run(110*time.Second, "idle", 2500, tdFinalText)
	if vars == nil || vars["rate"] != "150.0" || vars["avg_rate"] != "125.0" {
		t.Fatalf("idle refresh vars = %+v, want frozen rate=150.0 avg_rate=125.0", vars)
	}
}

func TestTPSModule_ColdStartIgnoresStaleTextStep(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)

	oldTD := &transcript.Data{
		LastStepIndex:     1,
		LastTPS:           95.0,
		AvgTPS:            95.0,
		LastOutputTokens:  950,
		LastDuration:      10 * time.Second,
		TotalOutputTokens: 950,
		TotalDuration:     10 * time.Second,
		LatestTextStep: transcript.TextPlannerStep{
			StepIndex:    3,
			OutputTokens: 1200,
			CreatedAt:    base,
		},
	}

	run := func(offset time.Duration, state string, td *transcript.Data) map[string]string {
		m := newTestTPSModule(dir, func() time.Time { return base.Add(offset) })
		ctx := &Context{
			Stdin: &stdin.Data{
				ConversationID: "conv-resumed",
				AgentState:     state,
			},
			Transcript: td,
		}
		if err := m.Collect(ctx); err != nil {
			t.Fatalf("Collect error: %v", err)
		}
		return m.Vars(ctx)
	}

	// Resuming a conversation 2 minutes after an old text reply must not fabricate
	// nowMs - CreatedAt; it should fall back to the transcript tool-step rate.
	if vars := run(2*time.Minute, "idle", oldTD); vars == nil || vars["rate"] != "95.0" {
		t.Fatalf("cold-start resumed vars = %+v, want rate=95.0", vars)
	}

	// Starting a new turn (working ticks before new step appears in transcript) must
	// still ignore the historical step 3 instead of measuring 122s -> 9.8 tok/s.
	if vars := run(2*time.Minute+1*time.Second, "working", oldTD); vars == nil || vars["rate"] != "95.0" || vars["avg_rate"] != "95.0" {
		t.Fatalf("working tick 1 after cold start vars = %+v, want rate=95.0 avg_rate=95.0", vars)
	}
	if vars := run(2*time.Minute+2*time.Second, "working", oldTD); vars == nil || vars["rate"] != "95.0" || vars["avg_rate"] != "95.0" {
		t.Fatalf("working tick 2 after cold start vars = %+v, want rate=95.0 avg_rate=95.0", vars)
	}

	// Once a genuinely new text step (step 5) finishes, it should be measured accurately.
	newTD := *oldTD
	newTD.LatestTextStep = transcript.TextPlannerStep{
		StepIndex:    5,
		OutputTokens: 800,
		CreatedAt:    base.Add(2*time.Minute + 2*time.Second),
	}
	if vars := run(2*time.Minute+10*time.Second, "idle", &newTD); vars == nil || vars["rate"] != "100.0" || vars["duration"] != "8.0s" {
		t.Fatalf("new step 5 vars = %+v, want rate=100.0 duration=8.0s", vars)
	}
}

func TestTPSModule_SubsecondCreatedAtAndPollDelayBound(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 11, 26, 8, 0, 10, 0, time.UTC)

	// Poll arrives 1.5s after whole-second CreatedAt (>= 1000ms) -> must NOT shift startMs by 1.5s.
	m1 := newTestTPSModule(dir, func() time.Time { return base.Add(1500 * time.Millisecond) })
	_ = m1.Collect(&Context{
		Stdin: &stdin.Data{ConversationID: "conv-subsec", AgentState: "working"},
	})
	m2 := newTestTPSModule(dir, func() time.Time { return base.Add(2000 * time.Millisecond) })
	ctx2 := &Context{
		Stdin: &stdin.Data{ConversationID: "conv-subsec", AgentState: "idle", ContextWindow: stdin.ContextWindow{TotalOutputTokens: 200}},
		Transcript: &transcript.Data{
			LatestTextStep: transcript.TextPlannerStep{
				StepIndex:    1,
				OutputTokens: 200,
				CreatedAt:    base, // 10.000s -> dur = 2.0s -> 100.0 tok/s (not 0.5s -> 400 tok/s)
			},
		},
	}
	_ = m2.Collect(ctx2)
	if vars := m2.Vars(ctx2); vars == nil || vars["rate"] != "100.0" || vars["duration"] != "2.0s" {
		t.Fatalf("poll-delay vars = %+v, want rate=100.0 duration=2.0s", vars)
	}
}

func TestTPSModule_ShortStepDoesNotOverwriteSubstantialTPSAndOLSDecode(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)

	// Synthetic session with true decode = 150 tok/s and TTFT = 6.0s:
	// dur = 6s + tokens/150s.
	td := &transcript.Data{
		LastStepIndex:     9,
		LastTPS:           45.0,
		LastOutputTokens:  450,
		LastDuration:      10 * time.Second,
		TotalOutputTokens: 16200,
		TotalDuration:     138 * time.Second,
		TPSSteps: []transcript.TPSStep{
			{StepIndex: 1, OutputTokens: 300, Duration: 8 * time.Second},   // 6 + 2
			{StepIndex: 3, OutputTokens: 1500, Duration: 16 * time.Second}, // 6 + 10
			{StepIndex: 5, OutputTokens: 4500, Duration: 36 * time.Second}, // 6 + 30
			{StepIndex: 7, OutputTokens: 9000, Duration: 66 * time.Second}, // 6 + 60 -> 136.4 tok/s (latest >= 1k)
			{StepIndex: 9, OutputTokens: 900, Duration: 12 * time.Second},  // 6 + 6  -> 75.0 tok/s (< 1k, must not overwrite)
		},
	}

	// Start working, then finish a tiny 150-token text reply in 10s (15.0 tok/s).
	m1 := newTestTPSModule(dir, func() time.Time { return base.Add(100 * time.Second) })
	_ = m1.Collect(&Context{
		Stdin:      &stdin.Data{ConversationID: "conv-ols", AgentState: "working", Model: &stdin.Model{DisplayName: "Model A"}},
		Transcript: td,
	})

	tdWithShortText := *td
	tdWithShortText.LatestTextStep = transcript.TextPlannerStep{
		StepIndex:    11,
		OutputTokens: 150,
		CreatedAt:    base.Add(100 * time.Second),
	}
	m2 := newTestTPSModule(dir, func() time.Time { return base.Add(107 * time.Second) }) // 6 + 1 = 7s
	ctx2 := &Context{
		Stdin:      &stdin.Data{ConversationID: "conv-ols", AgentState: "idle", Model: &stdin.Model{DisplayName: "Model A"}},
		Transcript: &tdWithShortText,
	}
	_ = m2.Collect(ctx2)
	vars := m2.Vars(ctx2)
	if vars == nil {
		t.Fatal("expected vars")
	}
	// Primary {tps} must stay on latest substantial step (step 7: 9000/66s = 136.4 tok/s),
	// NOT drop to step 11 (150/7s = 21.4 tok/s)!
	if vars["rate"] != "136.4" || vars["tokens"] != "9.0k" {
		t.Errorf("primary rate/tokens = (%q, %q), want (136.4, 9.0k)", vars["rate"], vars["tokens"])
	}
	// OLS decode should recover ~150.0 tok/s and TTFT ~6.0s.
	if vars["decode_rate"] != "150.0" || vars["ttft"] != "6.0s" {
		t.Errorf("OLS decode/ttft = (%q, %q), want (150.0, 6.0s)", vars["decode_rate"], vars["ttft"])
	}
	if vars["decode"] != ansi.Colored(ansi.Cyan, "150.0 tok/s") {
		t.Errorf("decode = %q", vars["decode"])
	}

	// Switching models resets live state cleanly so new model starts fresh.
	tdModelB := &transcript.Data{
		LastStepIndex:     13,
		LastTPS:           200.0,
		LastOutputTokens:  400,
		LastDuration:      2 * time.Second,
		TotalOutputTokens: 400,
		TotalDuration:     2 * time.Second,
		TPSSteps:          []transcript.TPSStep{{StepIndex: 13, OutputTokens: 400, Duration: 2 * time.Second, Model: "model-b"}},
	}
	m3 := newTestTPSModule(dir, func() time.Time { return base.Add(120 * time.Second) })
	ctx3 := &Context{
		Stdin:      &stdin.Data{ConversationID: "conv-ols", AgentState: "idle", Model: &stdin.Model{DisplayName: "Model B"}},
		Transcript: tdModelB,
	}
	_ = m3.Collect(ctx3)
	vars3 := m3.Vars(ctx3)
	if vars3 == nil || vars3["rate"] != "200.0" || vars3["decode_rate"] != "" || vars3["ttft"] != "" {
		t.Errorf("after model switch vars = %+v, want rate=200.0 and empty decode/ttft", vars3)
	}
}

func TestEstimateDecodeAndTTFT_Guardrails(t *testing.T) {
	// Too few samples -> rejected.
	if _, _, ok := estimateDecodeAndTTFT([]tpsStepRecord{
		{Tokens: 500, DurMs: 5000},
		{Tokens: 3000, DurMs: 25000},
	}); ok {
		t.Errorf("expected rejection with < 5 samples")
	}

	// Narrow token spread -> rejected.
	if _, _, ok := estimateDecodeAndTTFT([]tpsStepRecord{
		{Tokens: 1000, DurMs: 10000},
		{Tokens: 1100, DurMs: 11000},
		{Tokens: 1200, DurMs: 12000},
		{Tokens: 1300, DurMs: 13000},
		{Tokens: 1400, DurMs: 14000},
	}); ok {
		t.Errorf("expected rejection with narrow token spread")
	}

	// Uncorrelated noisy timings (low R^2) -> rejected.
	if _, _, ok := estimateDecodeAndTTFT([]tpsStepRecord{
		{Tokens: 200, DurMs: 30000},
		{Tokens: 1000, DurMs: 5000},
		{Tokens: 2500, DurMs: 40000},
		{Tokens: 5000, DurMs: 12000},
		{Tokens: 8000, DurMs: 18000},
	}); ok {
		t.Errorf("expected rejection on low R^2 noisy data")
	}
}
