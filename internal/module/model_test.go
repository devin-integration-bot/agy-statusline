package module

import (
	"testing"

	"github.com/mritd/agy-statusline/internal/stdin"
)

func TestModelModule(t *testing.T) {
	tests := []struct {
		name       string
		model      *stdin.Model
		wantNil    bool
		wantFull   string
		wantEffort string
	}{
		{
			name:    "nil_model",
			model:   nil,
			wantNil: true,
		},
		{
			name:       "appends_effort_when_missing",
			model:      &stdin.Model{ID: "gemini-4-neon", DisplayName: "Gemini 4 Neon", Effort: "high"},
			wantFull:   "Gemini 4 Neon (High)",
			wantEffort: "High",
		},
		{
			name:       "deduplicates_effort_already_in_display_name",
			model:      &stdin.Model{ID: "gemini-4-neon-high", DisplayName: "Gemini 4 Neon (High)", Effort: "high"},
			wantFull:   "Gemini 4 Neon (High)",
			wantEffort: "High",
		},
		{
			name:       "xhigh_effort",
			model:      &stdin.Model{ID: "gemini-4-hydrogen", DisplayName: "Gemini 4 Hydrogen", Effort: "xhigh"},
			wantFull:   "Gemini 4 Hydrogen (XHigh)",
			wantEffort: "XHigh",
		},
		{
			name:       "no_effort",
			model:      &stdin.Model{ID: "gemini-4-flash", DisplayName: "Gemini 4 Flash"},
			wantFull:   "Gemini 4 Flash",
			wantEffort: "",
		},
		{
			name:       "fallback_to_id",
			model:      &stdin.Model{ID: "claude-opus-4-7", Effort: "max"},
			wantFull:   "claude-opus-4-7 (Max)",
			wantEffort: "Max",
		},
	}

	m := NewModelModule()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &Context{Stdin: &stdin.Data{Model: tt.model}}
			vars := m.Vars(ctx)
			if tt.wantNil {
				if vars != nil {
					t.Fatalf("expected nil vars, got %+v", vars)
				}
				return
			}
			if vars["model"] != tt.wantFull {
				t.Errorf("model = %q, want %q", vars["model"], tt.wantFull)
			}
			if vars["effort"] != tt.wantEffort {
				t.Errorf("effort = %q, want %q", vars["effort"], tt.wantEffort)
			}
		})
	}
}
