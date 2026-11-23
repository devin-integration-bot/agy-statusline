package module

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/mritd/agy-statusline/internal/config"
	"github.com/mritd/agy-statusline/internal/stdin"
)

func TestGitModule_SkipsRevListByDefault(t *testing.T) {
	var (
		mu     sync.Mutex
		called [][]string
	)
	m := NewGitModule()
	m.runGit = func(cwd string, args ...string) (string, error) {
		mu.Lock()
		called = append(called, slices.Clone(args))
		mu.Unlock()
		switch args[0] {
		case "rev-parse":
			return "main\n", nil
		case "--no-optional-locks":
			return " M main.go\n", nil
		default:
			return "", errors.New("unexpected git call")
		}
	}

	ctx := &Context{CWD: "/tmp/repo", Config: config.Default()}
	if err := m.Collect(ctx); err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	if len(called) != 2 {
		t.Errorf("expected 2 git calls (no rev-list), got %d: %v", len(called), called)
	}
	vars := m.Vars(ctx)
	if !strings.Contains(vars["branch"], "main") || !strings.Contains(vars["dirty"], "*") {
		t.Errorf("vars = %+v", vars)
	}
}

func TestGitModule_FastPathFromStdinVCS(t *testing.T) {
	dirty := true
	m := NewGitModule()
	m.runGit = func(cwd string, args ...string) (string, error) {
		t.Fatalf("should not shell out to git when stdin VCS has branch & dirty: %v", args)
		return "", nil
	}

	ctx := &Context{
		CWD:    "/tmp/repo",
		Config: config.Default(),
		Stdin:  &stdin.Data{VCS: &stdin.VCS{Type: "git", Branch: "feat/fast", Dirty: &dirty}},
	}
	if err := m.Collect(ctx); err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	vars := m.Vars(ctx)
	if !strings.Contains(vars["branch"], "feat/fast") || !strings.Contains(vars["dirty"], "*") {
		t.Errorf("vars = %+v", vars)
	}
}

func TestParseAheadBehind(t *testing.T) {
	ahead, behind := parseAheadBehind("3\t5\n")
	if ahead != 5 || behind != 3 {
		t.Errorf("parseAheadBehind = (%d, %d), want (5, 3)", ahead, behind)
	}
}
