package module

import (
	"testing"

	"github.com/mritd/agy-statusline/internal/ansi"
	"github.com/mritd/agy-statusline/internal/stdin"
)

func TestTasksModule_Vars(t *testing.T) {
	m := NewTasksModule()

	if vars := m.Vars(&Context{}); vars != nil {
		t.Fatalf("nil stdin should return nil, got %+v", vars)
	}

	tests := []struct {
		name      string
		count     int
		wantRaw   string
		wantCount string
	}{
		{"negative_clamped", -1, "0", "0"},
		{"zero", 0, "0", "0"},
		{"positive", 4, "4", ansi.Colored(ansi.Green, "4")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vars := m.Vars(&Context{Stdin: &stdin.Data{TaskCount: tt.count}})
			if vars["raw"] != tt.wantRaw {
				t.Errorf("raw = %q, want %q", vars["raw"], tt.wantRaw)
			}
			if vars["count"] != tt.wantCount {
				t.Errorf("count = %q, want %q", vars["count"], tt.wantCount)
			}
		})
	}
}
