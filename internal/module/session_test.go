package module

import (
	"strings"
	"testing"

	"github.com/mritd/agy-statusline/internal/stdin"
)

func TestSessionModule(t *testing.T) {
	uuid := "73f8c8da-819a-4dfa-9bd3-575c931cc932"

	tests := []struct {
		name        string
		in          *stdin.Data
		wantNil     bool
		wantSession string
		wantTitle   string
		wantID      string
		wantShortID string
	}{
		{
			name:    "nil_stdin",
			in:      nil,
			wantNil: true,
		},
		{
			name:    "empty_fields",
			in:      &stdin.Data{},
			wantNil: true,
		},
		{
			name: "title_preferred_over_uuid",
			in: &stdin.Data{
				ConversationTitle: "Fix statusline jump",
				ConversationID:    uuid,
			},
			wantSession: "Fix statusline jump",
			wantTitle:   "Fix statusline jump",
			wantID:      uuid,
			wantShortID: "73f8c8da",
		},
		{
			name: "fallback_to_conversation_id",
			in: &stdin.Data{
				ConversationID: uuid,
			},
			wantSession: uuid,
			wantTitle:   "",
			wantID:      uuid,
			wantShortID: "73f8c8da",
		},
		{
			name: "fallback_to_session_id",
			in: &stdin.Data{
				SessionID: uuid,
			},
			wantSession: uuid,
			wantTitle:   "",
			wantID:      uuid,
			wantShortID: "73f8c8da",
		},
		{
			name: "title_only_without_id",
			in: &stdin.Data{
				ConversationTitle: "Renamed thread",
			},
			wantSession: "Renamed thread",
			wantTitle:   "Renamed thread",
			wantID:      "",
			wantShortID: "",
		},
		{
			name: "multiline_and_long_title_sanitized",
			in: &stdin.Data{
				ConversationTitle: "  line one\n\tline two " + strings.Repeat("字", 60),
				ConversationID:    uuid,
			},
			wantSession: "line one line two " + strings.Repeat("字", 42) + "...",
			wantTitle:   "line one line two " + strings.Repeat("字", 42) + "...",
			wantID:      uuid,
			wantShortID: "73f8c8da",
		},
	}

	m := NewSessionModule()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &Context{Stdin: tt.in}
			vars := m.Vars(ctx)
			if tt.wantNil {
				if vars != nil {
					t.Fatalf("expected nil vars, got %+v", vars)
				}
				return
			}
			if vars == nil {
				t.Fatalf("expected non-nil vars")
			}
			if vars["session"] != tt.wantSession {
				t.Errorf("session = %q, want %q", vars["session"], tt.wantSession)
			}
			if vars["title"] != tt.wantTitle {
				t.Errorf("title = %q, want %q", vars["title"], tt.wantTitle)
			}
			if vars["id"] != tt.wantID {
				t.Errorf("id = %q, want %q", vars["id"], tt.wantID)
			}
			if vars["short_id"] != tt.wantShortID {
				t.Errorf("short_id = %q, want %q", vars["short_id"], tt.wantShortID)
			}
		})
	}
}
