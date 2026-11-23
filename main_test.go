package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecute_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	transcriptPath := filepath.Join(dir, "transcript.jsonl")
	transcriptContent := strings.Join([]string{
		`{"step_index":0,"type":"USER_INPUT","status":"DONE","created_at":"2026-11-22T04:00:00Z","content":"hello"}`,
		`{"step_index":1,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-22T04:00:01Z","tool_calls":[{"name":"read_file","args":{"Path":"\"main.go\""}}]}`,
		`{"step_index":2,"type":"GENERIC","status":"DONE","created_at":"2026-11-22T04:00:02Z"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(transcriptPath, []byte(transcriptContent), 0o600); err != nil {
		t.Fatal(err)
	}

	payload := `{
		"cwd": "` + dir + `",
		"transcript_path": "` + transcriptPath + `",
		"model": {"display_name": "Gemini 4 Neon (High)"},
		"context_window": {
			"context_window_size": 1000000,
			"current_usage": {"input_tokens": 25000, "cache_read_input_tokens": 10000}
		},
		"quota": {
			"gemini-5h": {"remaining_fraction": 0.96, "reset_in_seconds": 8520},
			"gemini-weekly": {"remaining_fraction": 0.69, "reset_in_seconds": 223200}
		},
		"vcs": {"type": "git", "branch": "main", "dirty": true},
		"subagents": [{"name": "research", "role": "Review all uncommitted changes", "status": "running"}]
	}`

	out := execute(strings.NewReader(payload), cfgPath)
	lines := strings.Split(out, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines, got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "Ctx") || !strings.Contains(lines[0], "Usage") || !strings.Contains(lines[0], "Gemini 4 Neon (High)") || !strings.Contains(lines[0], "main") {
		t.Errorf("line 0 unexpected: %q", lines[0])
	}
	if strings.Index(lines[0], "Usage") > strings.Index(lines[0], "Gemini 4 Neon (High)") || strings.Index(lines[0], "Gemini 4 Neon (High)") > strings.Index(lines[0], "main") {
		t.Errorf("expected model between usage and git in line 0: %q", lines[0])
	}
	if !strings.Contains(lines[1], "read_file ×1") {
		t.Errorf("line 1 unexpected: %q", lines[1])
	}
	if !strings.Contains(lines[2], "Review all uncommitted changes...") {
		t.Errorf("line 2 unexpected: %q", lines[2])
	}
}

func TestExecute_InvalidJSON(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if out := execute(strings.NewReader("{bad"), cfgPath); out != "" {
		t.Errorf("expected empty output on malformed JSON, got %q", out)
	}
}
