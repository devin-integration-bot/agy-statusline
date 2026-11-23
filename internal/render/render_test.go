package render

import (
	"errors"
	"strings"
	"testing"

	"github.com/mritd/agy-statusline/internal/ansi"
	"github.com/mritd/agy-statusline/internal/config"
	"github.com/mritd/agy-statusline/internal/module"
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
