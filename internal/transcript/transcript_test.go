package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
