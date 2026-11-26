package transcript

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParse_AntigravityTranscript(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "transcript.jsonl")

	lines := []string{
		`{"step_index":0,"type":"USER_INPUT","status":"DONE","created_at":"2026-11-22T04:00:00Z","content":"turn 1"}`,
		`{"step_index":1,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-22T04:00:01Z","tool_calls":[{"name":"read_file","args":{"Path":"\"/Users/kovacs/project/main.go\""}},{"name":"run_command","args":{"CommandLine":"\"go test ./...\"","toolSummary":"\"Run tests\""}}]}`,
		`{"step_index":2,"type":"GENERIC","status":"DONE","created_at":"2026-11-22T04:00:02Z"}`,
		`{"step_index":3,"type":"GENERIC","status":"ERROR","error":"exit status 1","created_at":"2026-11-22T04:00:03Z"}`,
		`{"step_index":4,"type":"SYSTEM_MESSAGE","status":"DONE","created_at":"2026-11-22T04:00:04Z","content":"ignored"}`,
		// Turn 2 resets completed/error tools from turn 1, keeping session counts.
		`{"step_index":5,"type":"USER_INPUT","status":"DONE","created_at":"2026-11-22T04:01:00Z","content":"turn 2"}`,
		`{"step_index":6,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-22T04:01:01Z","tool_calls":[{"name":"edit_file","args":{"Path":"\"/Users/kovacs/project/app.go\""}},{"name":"invoke_subagent","args":{"Role":"\"Code Reviewer\"","TypeName":"\"research\"","Model":"\"pro\""}}]}`,
		`{"step_index":7,"type":"GENERIC","status":"RUNNING","created_at":"2026-11-22T04:01:02Z"}`,
		`{"step_index":8,"type":"GENERIC","status":"DONE","created_at":"2026-11-22T04:01:10Z"}`,
	}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	data, err := Parse(p, 0)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Only turn 2 tool should remain in current-turn Tools list.
	if len(data.Tools) != 1 {
		t.Fatalf("len(Tools) = %d, want 1 (%+v)", len(data.Tools), data.Tools)
	}
	if data.Tools[0].Name != "edit_file" || data.Tools[0].Status != StatusRunning {
		t.Errorf("Tools[0] = %+v", data.Tools[0])
	}
	if !strings.HasSuffix(data.Tools[0].Target, "app.go") {
		t.Errorf("Tools[0].Target = %q, want suffix app.go", data.Tools[0].Target)
	}

	// Session counts should span both turns and exclude invoke_subagent.
	if data.SessionToolCounts["read_file"] != 1 || data.SessionToolCounts["run_command"] != 1 || data.SessionToolCounts["edit_file"] != 1 {
		t.Errorf("SessionToolCounts = %+v", data.SessionToolCounts)
	}
	if _, exists := data.SessionToolCounts["invoke_subagent"]; exists {
		t.Errorf("invoke_subagent should not be counted in SessionToolCounts")
	}
	if len(data.SessionToolNames) != 3 || data.SessionToolNames[0] != "edit_file" {
		t.Errorf("SessionToolNames = %v", data.SessionToolNames)
	}

	// Agent entry parsed properly with unquoted args.
	if len(data.Agents) != 1 {
		t.Fatalf("len(Agents) = %d, want 1", len(data.Agents))
	}
	ag := data.Agents[0]
	if ag.Description != "Code Reviewer" || ag.Type != "research" || ag.Model != "pro" || ag.Status != StatusCompleted {
		t.Errorf("Agents[0] = %+v", ag)
	}
}

func TestParse_TailScanDropsOrphanRunning(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "transcript.jsonl")

	pad := strings.Repeat("x", 2048)
	lines := []string{
		`{"step_index":0,"type":"USER_INPUT","status":"DONE","created_at":"2026-11-22T04:00:00Z","content":"` + pad + `"}`,
		// Orphan running tool before next USER_INPUT (no GENERIC result)
		`{"step_index":1,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-22T04:00:01Z","tool_calls":[{"name":"read_file","args":{"Path":"\"a.go\""}}]}`,
		`{"step_index":2,"type":"USER_INPUT","status":"DONE","created_at":"2026-11-22T04:05:00Z","content":"new turn"}`,
		`{"step_index":3,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-22T04:05:01Z","tool_calls":[{"name":"write_file","args":{"Path":"\"b.go\""}}]}`,
	}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Seek inside line 0 so seeked=true and lines 1..3 are parsed.
	data, err := Parse(p, 700)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(data.Tools) != 1 || data.Tools[0].Name != "write_file" {
		t.Errorf("expected orphan read_file to be dropped, got %+v", data.Tools)
	}
}

func TestParse_SameStepRunningThenDoneAndCancelledTurn(t *testing.T) {
	p := filepath.Join(t.TempDir(), "transcript.jsonl")
	lines := []string{
		// Turn 1 cancelled before GENERIC arrives.
		`{"step_index":0,"type":"USER_INPUT","status":"DONE","created_at":"2026-11-22T04:00:00Z"}`,
		`{"step_index":1,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-22T04:00:01Z","tool_calls":[{"name":"run_command","args":{"CommandLine":"\"sleep 99\""}}]}`,
		// Turn 2 should not misalign with turn 1's cancelled tool.
		`{"step_index":2,"type":"USER_INPUT","status":"DONE","created_at":"2026-11-22T04:01:00Z"}`,
		`{"step_index":3,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-22T04:01:01Z","tool_calls":[{"name":"read_file","args":{"Path":"\"a.go\""}},{"name":"edit_file","args":{"Path":"\"b.go\""}}]}`,
		`{"step_index":4,"type":"GENERIC","status":"RUNNING","created_at":"2026-11-22T04:01:02Z"}`,
		`{"step_index":4,"type":"GENERIC","status":"DONE","created_at":"2026-11-22T04:01:03Z"}`,
		`{"step_index":5,"type":"GENERIC","status":"ERROR","error":"boom","created_at":"2026-11-22T04:01:04Z"}`,
	}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	data, err := Parse(p, 0)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(data.Tools) != 2 {
		t.Fatalf("len(Tools) = %d, want 2: %+v", len(data.Tools), data.Tools)
	}
	if data.Tools[0].Name != "read_file" || data.Tools[0].Status != StatusCompleted {
		t.Errorf("Tools[0] = %+v, want read_file completed", data.Tools[0])
	}
	if data.Tools[1].Name != "edit_file" || data.Tools[1].Status != StatusError {
		t.Errorf("Tools[1] = %+v, want edit_file error", data.Tools[1])
	}
}

func TestUnquoteArg(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"double_encoded", `"\"/tmp/a.go\""`, "/tmp/a.go"},
		{"single_string", `"/tmp/a.go"`, "/tmp/a.go"},
		{"plain_number", `5000`, "5000"},
		{"empty", ``, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := unquoteArg([]byte(tt.raw)); got != tt.want {
				t.Errorf("unquoteArg(%s) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestParse_TPSExtraction(t *testing.T) {
	p := filepath.Join(t.TempDir(), "transcript.jsonl")
	lines := []string{
		`{"step_index":0,"type":"USER_INPUT","status":"DONE","created_at":"2026-11-26T00:00:00Z","content":"hi"}`,
		// Step 1: 700 tokens across 2 streamed tool_calls; tool 1 created at 4s (updated DONE at 6s),
		// tool 2 created at 7s -> total generation duration is 7s (100.0 tok/s), counted once.
		`{"step_index":1,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-26T00:00:00Z","output_tokens":700,"tool_calls":[{"name":"read_file","args":{"Path":"\"a.go\""}},{"name":"read_file","args":{"Path":"\"b.go\""}}]}`,
		`{"step_index":2,"type":"GENERIC","status":"RUNNING","created_at":"2026-11-26T00:00:04Z"}`,
		`{"step_index":2,"type":"GENERIC","status":"DONE","created_at":"2026-11-26T00:00:06Z"}`,
		`{"step_index":3,"type":"GENERIC","status":"DONE","created_at":"2026-11-26T00:00:07Z"}`,
		// Cancelled planner step followed by USER_INPUT must not pair with next turn's GENERIC.
		`{"step_index":4,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-26T00:00:10Z","output_tokens":999,"tool_calls":[{"name":"run_command","args":{"CommandLine":"\"sleep 99\""}}]}`,
		`{"step_index":5,"type":"USER_INPUT","status":"DONE","created_at":"2026-11-26T00:02:00Z","content":"next"}`,
		// Step 6: 300 tokens in 3s -> 100.0 tok/s; total = 1000 tokens / 10s = 100.0 tok/s.
		`{"step_index":6,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-26T00:02:00Z","output_tokens":300,"tool_calls":[{"name":"edit_file","args":{"Path":"\"a.go\""}}]}`,
		`{"step_index":7,"type":"GENERIC","status":"DONE","created_at":"2026-11-26T00:02:03Z"}`,
		// Step 8: text-only final response (no tool_calls).
		`{"step_index":8,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-26T00:02:05Z","output_tokens":1200}`,
	}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	data, err := Parse(p, 0)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if data.LastStepIndex != 6 || data.LastOutputTokens != 300 || data.LastDuration != 3*time.Second {
		t.Errorf("last step = (idx=%d, %d tok, %v), want (6, 300, 3s)", data.LastStepIndex, data.LastOutputTokens, data.LastDuration)
	}
	if math.Abs(data.LastTPS-100.0) > 1e-6 {
		t.Errorf("LastTPS = %v, want 100.0", data.LastTPS)
	}
	if data.TotalOutputTokens != 1000 || data.TotalDuration != 10*time.Second {
		t.Errorf("totals = (%d tok, %v), want (1000, 10s)", data.TotalOutputTokens, data.TotalDuration)
	}
	if math.Abs(data.AvgTPS-100.0) > 1e-6 {
		t.Errorf("AvgTPS = %v, want 100.0", data.AvgTPS)
	}
	if data.LatestTextStep.StepIndex != 8 || data.LatestTextStep.OutputTokens != 1200 {
		t.Errorf("LatestTextStep = %+v, want step 8 with 1200 tokens", data.LatestTextStep)
	}
}

func TestParse_TPSIncompleteMultiToolBatchAtEOFNotPrematurelyFlushed(t *testing.T) {
	p := filepath.Join(t.TempDir(), "transcript.jsonl")
	lines := []string{
		`{"step_index":0,"type":"USER_INPUT","status":"DONE","created_at":"2026-11-26T00:00:00Z","content":"hi"}`,
		// Completed step 1: 500 tokens / 5s = 100 tok/s.
		`{"step_index":1,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-26T00:00:00Z","output_tokens":500,"tool_calls":[{"name":"read_file","args":{"Path":"\"a.go\""}}]}`,
		`{"step_index":2,"type":"GENERIC","status":"DONE","created_at":"2026-11-26T00:00:05Z"}`,
		// Step 3 emits 3 tools, but only the first GENERIC (step 4) has been written so far -> wait for step 6.
		`{"step_index":3,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-26T00:00:10Z","output_tokens":3000,"tool_calls":[{"name":"edit_file","args":{"Path":"\"a.go\""}},{"name":"edit_file","args":{"Path":"\"b.go\""}},{"name":"edit_file","args":{"Path":"\"c.go\""}}]}`,
		`{"step_index":4,"type":"GENERIC","status":"DONE","created_at":"2026-11-26T00:00:13Z"}`,
	}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	data, err := Parse(p, 0)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if data.LastStepIndex != 1 || math.Abs(data.LastTPS-100.0) > 1e-6 {
		t.Errorf("incomplete EOF batch should preserve step 1 (100 tok/s), got idx=%d tps=%v", data.LastStepIndex, data.LastTPS)
	}
}

func TestParse_TPSModelSwitchIsolatesHistory(t *testing.T) {
	p := filepath.Join(t.TempDir(), "transcript.jsonl")
	lines := []string{
		`{"step_index":0,"type":"USER_INPUT","status":"DONE","created_at":"2026-11-26T00:00:00Z","content":"hi"}`,
		`{"step_index":1,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-26T00:00:00Z","model":"model-a","output_tokens":2000,"tool_calls":[{"name":"read_file","args":{"Path":"\"a.go\""}}]}`,
		`{"step_index":2,"type":"GENERIC","status":"DONE","created_at":"2026-11-26T00:00:10Z"}`,
		// Switch to model-b -> previous model-a steps must be discarded from TPSSteps/totals.
		`{"step_index":3,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-26T00:00:20Z","model":"model-b","output_tokens":600,"tool_calls":[{"name":"read_file","args":{"Path":"\"b.go\""}}]}`,
		`{"step_index":4,"type":"GENERIC","status":"DONE","created_at":"2026-11-26T00:00:30Z"}`,
	}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	data, err := Parse(p, 0)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(data.TPSSteps) != 1 || data.TPSSteps[0].Model != "model-b" {
		t.Fatalf("TPSSteps = %+v, want only model-b step", data.TPSSteps)
	}
	if data.TotalOutputTokens != 600 || data.TotalDuration != 10*time.Second || math.Abs(data.AvgTPS-60.0) > 1e-6 {
		t.Errorf("model-b totals = (%d, %v, avg=%v), want (600, 10s, 60.0)", data.TotalOutputTokens, data.TotalDuration, data.AvgTPS)
	}
}

func TestParse_TPSOutOfOrderGenericAndMinDuration(t *testing.T) {
	p := filepath.Join(t.TempDir(), "transcript.jsonl")
	lines := []string{
		`{"step_index":0,"type":"USER_INPUT","status":"DONE","created_at":"2026-11-26T00:00:00Z","content":"hi"}`,
		// Step 1 starts an async tool (step 2).
		`{"step_index":1,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-26T00:00:00Z","output_tokens":200,"tool_calls":[{"name":"run_command","args":{"CommandLine":"\"long\""}}]}`,
		`{"step_index":2,"type":"GENERIC","status":"RUNNING","created_at":"2026-11-26T00:00:02Z"}`,
		// Step 3 emits a new planner response; step 2 finishes late BEFORE step 4 arrives.
		`{"step_index":3,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-26T00:00:05Z","output_tokens":500,"tool_calls":[{"name":"read_file","args":{"Path":"\"a.go\""}}]}`,
		`{"step_index":2,"type":"GENERIC","status":"DONE","created_at":"2026-11-26T00:00:06Z"}`,
		`{"step_index":4,"type":"GENERIC","status":"DONE","created_at":"2026-11-26T00:00:10Z"}`,
		// Step 5 has a sub-100ms delta to step 6 -> must be ignored.
		`{"step_index":5,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-26T00:00:11.000Z","output_tokens":900,"tool_calls":[{"name":"read_file","args":{"Path":"\"b.go\""}}]}`,
		`{"step_index":6,"type":"GENERIC","status":"DONE","created_at":"2026-11-26T00:00:11.050Z"}`,
	}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	data, err := Parse(p, 0)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	// Step 3 (500 tok / 5s = 100 tok/s) must be preserved; step 5 (50ms) must be ignored.
	if data.LastOutputTokens != 500 || data.LastDuration != 5*time.Second {
		t.Errorf("last step = (%d tok, %v), want (500, 5s)", data.LastOutputTokens, data.LastDuration)
	}
	if math.Abs(data.LastTPS-100.0) > 1e-6 {
		t.Errorf("LastTPS = %v, want 100.0", data.LastTPS)
	}
	if data.TotalOutputTokens != 700 || data.TotalDuration != 7*time.Second {
		t.Errorf("totals = (%d tok, %v), want (700, 7s)", data.TotalOutputTokens, data.TotalDuration)
	}
}
