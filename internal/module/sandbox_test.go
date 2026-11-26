package module

import (
	"testing"

	"github.com/mritd/agy-statusline/internal/ansi"
	"github.com/mritd/agy-statusline/internal/stdin"
)

func TestSandboxModule_Vars(t *testing.T) {
	m := NewSandboxModule()

	if vars := m.Vars(&Context{}); vars != nil {
		t.Fatalf("nil stdin should return nil, got %+v", vars)
	}

	tests := []struct {
		name        string
		sandbox     *stdin.Sandbox
		wantRaw     string
		wantSandbox string
		wantStatus  string
	}{
		{
			name:        "nil_sandbox_is_off",
			sandbox:     nil,
			wantRaw:     "off",
			wantSandbox: ansi.DimText("sandbox off"),
			wantStatus:  ansi.DimText("off"),
		},
		{
			name:        "disabled",
			sandbox:     &stdin.Sandbox{Enabled: false},
			wantRaw:     "off",
			wantSandbox: ansi.DimText("sandbox off"),
			wantStatus:  ansi.DimText("off"),
		},
		{
			name:        "enabled",
			sandbox:     &stdin.Sandbox{Enabled: true},
			wantRaw:     "on",
			wantSandbox: ansi.DimText("sandbox ") + ansi.Colored(ansi.BoldBrightGreen, "ON"),
			wantStatus:  ansi.Colored(ansi.BoldBrightGreen, "ON"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vars := m.Vars(&Context{Stdin: &stdin.Data{Sandbox: tt.sandbox}})
			if vars["raw"] != tt.wantRaw {
				t.Errorf("raw = %q, want %q", vars["raw"], tt.wantRaw)
			}
			if vars["sandbox"] != tt.wantSandbox {
				t.Errorf("sandbox = %q, want %q", vars["sandbox"], tt.wantSandbox)
			}
			if vars["status"] != tt.wantStatus {
				t.Errorf("status = %q, want %q", vars["status"], tt.wantStatus)
			}
		})
	}
}
