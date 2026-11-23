package module

import (
	"testing"

	"github.com/mritd/agy-statusline/internal/stdin"
)

func TestProjectModule(t *testing.T) {
	m := NewProjectModule()
	ctx := &Context{
		CWD: "/Users/kovacs/gopath/src/github.com/mritd/agy-statusline",
		Stdin: &stdin.Data{
			Model:         &stdin.Model{DisplayName: "Gemini 4 Neon (High)", Effort: "high"},
			AgentState:    "working",
			CycleMode:     "accept-edits",
			Sandbox:       &stdin.Sandbox{Enabled: true},
			Cost:          &stdin.Cost{TotalUSD: 0.42},
			TaskCount:     2,
			ArtifactCount: 3,
		},
	}
	vars := m.Vars(ctx)
	if vars["model"] != "Gemini 4 Neon (High)" || vars["effort"] != "high" || vars["path"] != "agy-statusline" {
		t.Errorf("basic vars = %+v", vars)
	}
	if vars["state"] != "working" || vars["mode"] != "accept-edits" || vars["sandbox"] != "sandbox" {
		t.Errorf("state vars = %+v", vars)
	}
	if vars["cost"] != "$0.42" || vars["tasks"] != "2" || vars["artifacts"] != "3" {
		t.Errorf("count/cost vars = %+v", vars)
	}
}
