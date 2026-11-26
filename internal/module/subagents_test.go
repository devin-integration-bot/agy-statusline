package module

import (
	"testing"

	"github.com/mritd/agy-statusline/internal/ansi"
	"github.com/mritd/agy-statusline/internal/stdin"
)

func TestSubagentsModule_Vars(t *testing.T) {
	m := NewSubagentsModule()

	if vars := m.Vars(&Context{}); vars != nil {
		t.Fatalf("nil stdin should return nil, got %+v", vars)
	}

	empty := m.Vars(&Context{Stdin: &stdin.Data{}})
	if empty["count"] != "0" || empty["running"] != "0" || empty["raw"] != "0" || empty["raw_running"] != "0" {
		t.Errorf("empty vars = %+v", empty)
	}

	active := m.Vars(&Context{
		Stdin: &stdin.Data{
			Subagents: []stdin.Subagent{
				{Name: "research", Status: "running"},
				{Name: "self", Status: "completed"},
			},
		},
	})
	if active["count"] != ansi.Colored(ansi.Green, "2") || active["running"] != ansi.Colored(ansi.Green, "1") || active["raw"] != "2" || active["raw_running"] != "1" {
		t.Errorf("active vars = %+v", active)
	}
}
