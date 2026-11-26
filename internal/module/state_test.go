package module

import (
	"strings"
	"testing"

	"github.com/mritd/agy-statusline/internal/ansi"
	"github.com/mritd/agy-statusline/internal/config"
	"github.com/mritd/agy-statusline/internal/stdin"
)

func TestStateModule_Vars(t *testing.T) {
	m := NewStateModule()

	if vars := m.Vars(&Context{Config: config.Default()}); vars != nil {
		t.Fatalf("nil stdin should return nil, got %+v", vars)
	}

	tests := []struct {
		name      string
		rawState  string
		wantRaw   string
		wantLabel string
		wantIcon  string
		wantColor string
	}{
		{"empty_defaults_idle", "", "idle", "READY", "●", ansi.BoldBrightGreen},
		{"idle", "idle", "idle", "READY", "●", ansi.BoldBrightGreen},
		{"thinking", "thinking", "thinking", "THINKING", "◆", ansi.BoldBrightYellow},
		{"working", "working", "working", "WORKING", "⚙", ansi.BoldBrightCyan},
		{"tool_use", "tool_use", "tool_use", "TOOL", "🔧", ansi.BoldBrightMagenta},
		{"unknown", "waiting_input", "waiting_input", "WAITING_INPUT", "⏳", ansi.BoldWhite},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &Context{
				Config: config.Default(),
				Stdin:  &stdin.Data{AgentState: tt.rawState},
			}
			vars := m.Vars(ctx)
			if vars["raw"] != tt.wantRaw {
				t.Errorf("raw = %q, want %q", vars["raw"], tt.wantRaw)
			}
			if vars["label"] != tt.wantLabel {
				t.Errorf("label = %q, want %q", vars["label"], tt.wantLabel)
			}
			if vars["icon"] != tt.wantIcon {
				t.Errorf("icon = %q, want %q", vars["icon"], tt.wantIcon)
			}
			wantState := ansi.Colored(tt.wantColor, tt.wantIcon+" "+tt.wantLabel)
			if vars["state"] != wantState {
				t.Errorf("state = %q, want %q", vars["state"], wantState)
			}
		})
	}
}

func TestStateModule_CustomIconOverride(t *testing.T) {
	cfg := config.Default()
	cfg.Icons["state_working"] = "*"
	m := NewStateModule()
	vars := m.Vars(&Context{
		Config: cfg,
		Stdin:  &stdin.Data{AgentState: "working"},
	})
	if !strings.Contains(vars["state"], "* WORKING") {
		t.Errorf("state = %q, want custom icon", vars["state"])
	}
}
