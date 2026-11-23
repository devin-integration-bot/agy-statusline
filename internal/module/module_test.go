package module

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestExpandFormat(t *testing.T) {
	vars := map[string]string{
		"a": "hello",
		"b": "world",
	}
	tests := []struct {
		name   string
		format string
		want   string
	}{
		{"simple", "{a} {b}", "hello world"},
		{"unknown_var", "{a} {missing}!", "hello !"},
		{"escaped_braces", "{{a}} = {a}", "{a} = hello"},
		{"unclosed", "{a", "{a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExpandFormat(tt.format, vars); got != tt.want {
				t.Errorf("ExpandFormat(%q) = %q, want %q", tt.format, got, tt.want)
			}
		})
	}
}

func TestRegistry_DeduplicatesEnabled(t *testing.T) {
	reg := NewRegistry()
	reg.Register(NewProjectModule())
	reg.Register(NewGitModule())
	got := reg.Enabled([]string{"project", "git", "project", "git"})
	if len(got) != 2 || got[0].Name() != "project" || got[1].Name() != "git" {
		t.Errorf("Enabled deduplication failed: got %d modules", len(got))
	}
}

func TestContext_LoadTranscriptConcurrent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "transcript.jsonl")
	line := `{"step_index":1,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-22T04:00:01Z","tool_calls":[{"name":"read_file","args":{"Path":"\"a.go\""}}]}` + "\n"
	if err := os.WriteFile(p, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := &Context{TranscriptPath: p}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			td := ctx.LoadTranscript()
			if td == nil || len(td.Tools) != 1 {
				t.Errorf("unexpected transcript data: %+v", td)
			}
		}()
	}
	wg.Wait()
}
