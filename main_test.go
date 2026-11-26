package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecute_EndToEnd(t *testing.T) {
	t.Setenv("AGY_CONFIG_DIR", t.TempDir())
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	transcriptPath := filepath.Join(dir, "transcript.jsonl")
	transcriptContent := strings.Join([]string{
		`{"step_index":0,"type":"USER_INPUT","status":"DONE","created_at":"2026-11-22T04:00:00Z","content":"hello"}`,
		`{"step_index":1,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-22T04:00:01Z","output_tokens":425,"tool_calls":[{"name":"read_file","args":{"Path":"\"main.go\""}}]}`,
		`{"step_index":2,"type":"GENERIC","status":"DONE","created_at":"2026-11-22T04:00:06Z"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(transcriptPath, []byte(transcriptContent), 0o600); err != nil {
		t.Fatal(err)
	}

	payload := `{
		"cwd": "` + dir + `",
		"conversation_id": "73f8c8da-819a-4dfa-9bd3-575c931cc932",
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
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines, got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "Ctx") || !strings.Contains(lines[0], "Usage") || !strings.Contains(lines[0], "Gemini 4 Neon (High)") || !strings.Contains(lines[0], "85.0 tok/s") || !strings.Contains(lines[0], "main") {
		t.Errorf("line 0 unexpected: %q", lines[0])
	}
	if strings.Index(lines[0], "Usage") > strings.Index(lines[0], "Gemini 4 Neon (High)") ||
		strings.Index(lines[0], "Gemini 4 Neon (High)") > strings.Index(lines[0], "85.0 tok/s") ||
		strings.Index(lines[0], "85.0 tok/s") > strings.Index(lines[0], "main") {
		t.Errorf("expected order usage -> model -> tps -> git in line 0: %q", lines[0])
	}
	if !strings.Contains(lines[1], "read_file ×1") {
		t.Errorf("line 1 unexpected: %q", lines[1])
	}
	if !strings.Contains(lines[2], "Review all uncommitted changes...") {
		t.Errorf("line 2 unexpected: %q", lines[2])
	}
	if !strings.HasSuffix(lines[3], " | 73f8c8da-819a-4dfa-9bd3-575c931cc932") {
		t.Errorf("line 3 expected session UUID at end: %q", lines[3])
	}
}

func TestExecute_InvalidJSON(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if out := execute(strings.NewReader("{bad"), cfgPath); out != "" {
		t.Errorf("expected empty output on malformed JSON, got %q", out)
	}
}

func TestExecute_TranscriptPathFallbackFromAgyDir(t *testing.T) {
	agyDir := t.TempDir()
	t.Setenv("AGY_CONFIG_DIR", agyDir)

	convID := "73f8c8da-819a-4dfa-9bd3-575c931cc932"
	transcriptPath := filepath.Join(agyDir, "brain", convID, ".system_generated", "logs", "transcript.jsonl")
	if err := os.MkdirAll(filepath.Dir(transcriptPath), 0o700); err != nil {
		t.Fatal(err)
	}
	lines := strings.Join([]string{
		`{"step_index":0,"type":"USER_INPUT","status":"DONE","created_at":"2026-11-22T04:00:00Z","content":"hi"}`,
		`{"step_index":1,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-22T04:00:01Z","tool_calls":[{"name":"read_file","args":{"Path":"\"main.go\""}}]}`,
		`{"step_index":2,"type":"GENERIC","status":"DONE","created_at":"2026-11-22T04:00:02Z"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(transcriptPath, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	payload := `{
		"conversation_id": "` + convID + `",
		"transcript_path": "/nonexistent/.gemini/antigravity/brain/` + convID + `/.system_generated/logs/transcript.jsonl"
	}`

	out := execute(strings.NewReader(payload), cfgPath)
	if !strings.Contains(out, "read_file ×1") {
		t.Errorf("expected tools line via AGY_CONFIG_DIR fallback, got:\n%s", out)
	}
}

func TestExecute_OfficialModulesDisabledByDefaultAndOptIn(t *testing.T) {
	t.Setenv("AGY_CONFIG_DIR", t.TempDir())
	payload := `{
		"agent_state": "working",
		"model": {"display_name": "Gemini 4 Neon"},
		"context_window": {"context_window_size": 1000000, "used_percentage": 14.2},
		"vcs": {"branch": "main", "dirty": true},
		"sandbox": {"enabled": true},
		"artifact_count": 3,
		"task_count": 2,
		"subagents": [{"name": "research", "role": "Search", "status": "completed"}],
		"terminal_width": 100
	}`

	// Default config should NOT output the new opt-in modules.
	defaultCfg := filepath.Join(t.TempDir(), "default.json")
	defaultOut := execute(strings.NewReader(payload), defaultCfg)
	for _, forbidden := range []string{"WORKING", "sandbox", "artifacts", "subagents", "tasks"} {
		if strings.Contains(defaultOut, forbidden) {
			t.Errorf("default output should not contain %q, got:\n%s", forbidden, defaultOut)
		}
	}

	// Explicit opt-in config with fine bar, exact percent, count agents, and responsive layout.
	optInCfg := filepath.Join(t.TempDir(), "optin.json")
	cfgJSON := `{
		"bar_style": "fine",
		"context": {"format": "ctx {bar} {percent_exact}", "bar_width": 15, "context_limit": 0},
		"agents": {"format": "subagents {count}"},
		"responsive": [
			{
				"min_width": 120,
				"modules": ["state", "model", "git", "context", "artifacts", "agents", "tasks", "sandbox"],
				"newline": [],
				"separator": " · "
			},
			{
				"min_width": 80,
				"modules": ["state", "model", "git", "context", "artifacts", "agents", "tasks", "sandbox"],
				"newline": ["context"],
				"separator": " · ",
				"line_prefixes": ["╭─ ", "╰─ "]
			},
			{
				"min_width": 0,
				"modules": ["state", "model", "context", "tasks"],
				"newline": ["context"],
				"separator": " · "
			}
		]
	}`
	if err := os.WriteFile(optInCfg, []byte(cfgJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	medOut := execute(strings.NewReader(payload), optInCfg)
	medLines := strings.Split(medOut, "\n")
	if len(medLines) != 2 {
		t.Fatalf("expected 2 lines for medium width, got %d:\n%s", len(medLines), medOut)
	}
	if !strings.HasPrefix(medLines[0], "╭─ ") || !strings.Contains(medLines[0], "WORKING") || !strings.Contains(medLines[0], "main") {
		t.Errorf("medium line 0 unexpected: %q", medLines[0])
	}
	for _, want := range []string{"╰─ ", "14.2%", "artifacts", "subagents", "tasks", "sandbox", "ON"} {
		if !strings.Contains(medLines[1], want) {
			t.Errorf("medium line 1 missing %q: %q", want, medLines[1])
		}
	}

	widePayload := strings.Replace(payload, `"terminal_width": 100`, `"terminal_width": 140`, 1)
	wideOut := execute(strings.NewReader(widePayload), optInCfg)
	if strings.Contains(wideOut, "\n") {
		t.Errorf("expected single line for wide width, got:\n%s", wideOut)
	}

	narrowPayload := strings.Replace(payload, `"terminal_width": 100`, `"terminal_width": 60`, 1)
	narrowOut := execute(strings.NewReader(narrowPayload), optInCfg)
	if strings.Contains(narrowOut, "sandbox") || strings.Contains(narrowOut, "artifacts") {
		t.Errorf("narrow layout should hide sandbox/artifacts, got:\n%s", narrowOut)
	}
}
