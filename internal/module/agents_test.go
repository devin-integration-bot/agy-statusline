package module

import (
	"strings"
	"testing"
	"time"

	"github.com/mritd/agy-statusline/internal/ansi"
	"github.com/mritd/agy-statusline/internal/config"
	"github.com/mritd/agy-statusline/internal/stdin"
	"github.com/mritd/agy-statusline/internal/transcript"
)

func TestAgentsModule_StdinSubagentsPreferred(t *testing.T) {
	m := NewAgentsModule()
	ctx := &Context{
		Config: config.Default(),
		Stdin: &stdin.Data{
			Subagents: []stdin.Subagent{
				{Name: "research", Role: "Completed Task", Status: "completed"},
				{Name: "self", Role: "Active Reviewer", Status: "running"},
			},
		},
	}
	if err := m.Collect(ctx); err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	vars := m.Vars(ctx)
	if !strings.Contains(vars["running"], ansi.Cyan) || !strings.Contains(vars["running"], "Active Reviewer...") {
		t.Errorf("running = %q", vars["running"])
	}
	if vars["completed"] != "" {
		t.Errorf("completed should be hidden while agents are running, got %q", vars["completed"])
	}
}

func TestAgentsModule_TranscriptFallbackLatestCompleted(t *testing.T) {
	start := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	m := NewAgentsModule()
	ctx := &Context{
		Config: config.Default(),
		Stdin:  &stdin.Data{},
		Transcript: &transcript.Data{
			Agents: []transcript.AgentEntry{
				{Description: "First", Status: transcript.StatusCompleted, StartTime: start, EndTime: start.Add(10 * time.Second)},
				{Description: "Latest", Status: transcript.StatusCompleted, StartTime: start, EndTime: start.Add(65 * time.Second)},
			},
		},
	}
	if err := m.Collect(ctx); err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	vars := m.Vars(ctx)
	if !strings.Contains(vars["completed"], "Latest (1m 5s)") {
		t.Errorf("completed = %q", vars["completed"])
	}
	if strings.Contains(vars["completed"], "First") {
		t.Errorf("expected only latest completed agent, got %q", vars["completed"])
	}
}
