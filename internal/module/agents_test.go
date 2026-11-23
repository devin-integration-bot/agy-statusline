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
	if !strings.Contains(vars["completed"], "✓ Latest (1m 5s)") {
		t.Errorf("completed = %q", vars["completed"])
	}
	if strings.Contains(vars["completed"], "First") {
		t.Errorf("expected only latest completed agent, got %q", vars["completed"])
	}
	if got := ExpandFormat(m.DefaultFormat(), vars); got != "" {
		t.Errorf("default format should hide completed agents, got %q", got)
	}
}

func TestAgentsModule_StdinSubagentsDeterministicOrder(t *testing.T) {
	cfg := config.Default()

	orderA := []stdin.Subagent{
		{Name: "research", Role: "Bravo Reviewer", Status: "completed"},
		{Name: "self", Role: "Alpha Reviewer", Status: "completed"},
	}
	orderB := []stdin.Subagent{
		{Name: "self", Role: "Alpha Reviewer", Status: "completed"},
		{Name: "research", Role: "Bravo Reviewer", Status: "completed"},
	}

	mA := NewAgentsModule()
	_ = mA.Collect(&Context{Config: cfg, Stdin: &stdin.Data{Subagents: orderA}})
	varsA := mA.Vars(&Context{Config: cfg})

	mB := NewAgentsModule()
	_ = mB.Collect(&Context{Config: cfg, Stdin: &stdin.Data{Subagents: orderB}})
	varsB := mB.Vars(&Context{Config: cfg})

	if varsA["completed"] != varsB["completed"] {
		t.Errorf("completed order should be deterministic: %q vs %q", varsA["completed"], varsB["completed"])
	}
	if !strings.HasPrefix(varsA["completed"], "✓ ") {
		t.Errorf("completed should use completed icon, got %q", varsA["completed"])
	}
	if got := ExpandFormat(mA.DefaultFormat(), varsA); got != "" {
		t.Errorf("default format should hide completed stdin subagents, got %q", got)
	}

	runA := []stdin.Subagent{
		{Name: "self", Role: "Zulu Task", Status: "running"},
		{Name: "research", Role: "Alpha Task", Status: "running"},
	}
	runB := []stdin.Subagent{
		{Name: "research", Role: "Alpha Task", Status: "running"},
		{Name: "self", Role: "Zulu Task", Status: "running"},
	}

	mrA := NewAgentsModule()
	_ = mrA.Collect(&Context{Config: cfg, Stdin: &stdin.Data{Subagents: runA}})
	rvA := mrA.Vars(&Context{Config: cfg})

	mrB := NewAgentsModule()
	_ = mrB.Collect(&Context{Config: cfg, Stdin: &stdin.Data{Subagents: runB}})
	rvB := mrB.Vars(&Context{Config: cfg})

	if rvA["running"] != rvB["running"] {
		t.Errorf("running order should be deterministic: %q vs %q", rvA["running"], rvB["running"])
	}
}

func TestAgentsModule_NoDoubleEllipsisWhenTruncated(t *testing.T) {
	cfg := config.Default()
	longRole := "Perform an adversarial functional, regression, and compatibility review of all uncommitted changes"

	m := NewAgentsModule()
	_ = m.Collect(&Context{
		Config: cfg,
		Stdin: &stdin.Data{
			Subagents: []stdin.Subagent{
				{Name: "self", Role: longRole, Status: "running"},
			},
		},
	})
	vars := m.Vars(&Context{Config: cfg})
	if strings.Contains(vars["running"], "......") {
		t.Errorf("running should not contain double ellipsis, got %q", vars["running"])
	}
	if !strings.Contains(vars["running"], "...") {
		t.Errorf("running should still end with ellipsis, got %q", vars["running"])
	}
}
