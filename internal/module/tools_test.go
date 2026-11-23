package module

import (
	"strings"
	"testing"

	"github.com/mritd/agy-statusline/internal/ansi"
	"github.com/mritd/agy-statusline/internal/config"
	"github.com/mritd/agy-statusline/internal/transcript"
)

func TestToolsModule(t *testing.T) {
	m := NewToolsModule()
	ctx := &Context{
		Config: config.Default(),
		Transcript: &transcript.Data{
			Tools: []transcript.ToolEntry{
				{Name: "read_file", Target: "...config.go", Status: transcript.StatusRunning},
				{Name: "edit_file", Status: transcript.StatusCompleted},
				{Name: "run_command", Status: transcript.StatusError},
			},
			SessionToolNames: []string{"read_file", "edit_file", "run_command", "write_file"},
			SessionToolCounts: map[string]int{
				"read_file":   5,
				"edit_file":   3,
				"run_command": 2,
				"write_file":  4,
			},
		},
	}

	if err := m.Collect(ctx); err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	vars := m.Vars(ctx)
	if !strings.Contains(vars["running"], "read_file: ...config.go") {
		t.Errorf("running = %q", vars["running"])
	}
	if !strings.Contains(vars["completed"], ansi.Green) || !strings.Contains(vars["completed"], "edit_file ×3") {
		t.Errorf("completed = %q", vars["completed"])
	}
	if !strings.Contains(vars["completed"], "write_file ×4") {
		t.Errorf("expected inactive session tool write_file in completed: %q", vars["completed"])
	}
	if !strings.Contains(vars["errors"], ansi.Red) || !strings.Contains(vars["errors"], "run_command ×2") {
		t.Errorf("errors = %q", vars["errors"])
	}
	if vars["summary"] == "" {
		t.Errorf("summary should not be empty")
	}
}
