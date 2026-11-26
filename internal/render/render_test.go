package render

import (
	"errors"
	"strings"
	"testing"

	"github.com/mritd/agy-statusline/internal/ansi"
	"github.com/mritd/agy-statusline/internal/config"
	"github.com/mritd/agy-statusline/internal/module"
	"github.com/mritd/agy-statusline/internal/stdin"
)

type stubModule struct {
	name   string
	err    error
	vars   map[string]string
	format string
}

func (s *stubModule) Name() string                           { return s.name }
func (s *stubModule) Collect(*module.Context) error          { return s.err }
func (s *stubModule) Vars(*module.Context) map[string]string { return s.vars }
func (s *stubModule) DefaultFormat() string                  { return s.format }

func TestBar(t *testing.T) {
	bar := NewContextBar("◆", "◇")(50, 10)
	if !strings.Contains(bar, "◆◆◆◆◆") || !strings.Contains(bar, "◇◇◇◇◇") {
		t.Errorf("unexpected bar: %q", bar)
	}
	if !strings.HasPrefix(bar, ansi.Green) {
		t.Errorf("expected green prefix, got %q", bar)
	}
	if got := Bar(50, -1, "◆", "◇", ansi.ContextColor); got != "" {
		t.Errorf("negative width should return empty string, got %q", got)
	}
}

func TestRender_MultilineAndErrorSkip(t *testing.T) {
	cfg := config.Default()
	cfg.Modules = []string{"a", "b", "c", "d"}
	cfg.Newline = []string{"c"}
	cfg.Separator = " | "

	reg := module.NewRegistry()
	reg.Register(&stubModule{name: "a", vars: map[string]string{"v": "A"}, format: "{v}"})
	reg.Register(&stubModule{name: "b", err: errors.New("boom"), vars: map[string]string{"v": "B"}, format: "{v}"})
	reg.Register(&stubModule{name: "c", vars: map[string]string{"v": "C"}, format: "{v}"})
	reg.Register(&stubModule{name: "d", vars: map[string]string{"v": "D"}, format: "{v}"})

	got := Render(&module.Context{Config: cfg}, reg, cfg)
	want := "A\nC | D"
	if got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestStepBar_FineStyle(t *testing.T) {
	fine := []string{"█", "▓", "▒", "░", "·"}
	fn := NewContextBar(fine...)

	tests := []struct {
		name       string
		pct, width int
		wantFilled string
		wantEmpty  string
	}{
		{"zero", 0, 10, "", "··········"},
		{"partial_quarter", 13, 10, "█░", "········"},
		{"partial_half", 15, 10, "█▒", "········"},
		{"partial_three_quarter", 18, 10, "█▓", "········"},
		{"full", 100, 10, "██████████", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fn(tt.pct, tt.width)
			want := ansi.ContextColor(tt.pct) + tt.wantFilled + ansi.Reset + ansi.Dim + tt.wantEmpty + ansi.Reset
			if got != want {
				t.Errorf("StepBar(%d, %d) = %q, want %q", tt.pct, tt.width, got, want)
			}
		})
	}
}

func TestRender_ResponsiveAndLinePrefixes(t *testing.T) {
	sep := " · "
	cfg := config.Default()
	cfg.Responsive = []config.ResponsiveLayout{
		{
			MinWidth:  120,
			Modules:   []string{"a", "b", "c"},
			Newline:   []string{},
			Separator: &sep,
		},
		{
			MinWidth:     80,
			Modules:      []string{"a", "b", "c"},
			Newline:      []string{"c"},
			Separator:    &sep,
			LinePrefixes: []string{"╭─ ", "╰─ "},
		},
		{
			MinWidth:  0,
			Modules:   []string{"a", "c"},
			Newline:   []string{"c"},
			Separator: &sep,
		},
	}

	reg := module.NewRegistry()
	reg.Register(&stubModule{name: "a", vars: map[string]string{"v": "A"}, format: "{v}"})
	reg.Register(&stubModule{name: "b", vars: map[string]string{"v": "B"}, format: "{v}"})
	reg.Register(&stubModule{name: "c", vars: map[string]string{"v": "C"}, format: "{v}"})

	if got := Render(&module.Context{Config: cfg, Stdin: &stdin.Data{TerminalWidth: 130}}, reg, cfg); got != "A · B · C" {
		t.Errorf("wide Render() = %q", got)
	}
	if got := Render(&module.Context{Config: cfg, Stdin: &stdin.Data{TerminalWidth: 90}}, reg, cfg); got != "╭─ A · B\n╰─ C" {
		t.Errorf("medium Render() = %q", got)
	}
	if got := Render(&module.Context{Config: cfg, Stdin: &stdin.Data{TerminalWidth: 60}}, reg, cfg); got != "A\nC" {
		t.Errorf("narrow Render() = %q", got)
	}
}
