package module

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mritd/agy-statusline/internal/ansi"
	"github.com/mritd/agy-statusline/internal/debug"
)

// gitTimeout bounds git subprocesses tightly so statusline stays far below
// Antigravity CLI's 5-second command timeout even under heavy machine load.
const gitTimeout = 500 * time.Millisecond

type GitModule struct {
	branch string
	dirty  bool
	ahead  int
	behind int

	runGit func(cwd string, args ...string) (string, error)
}

func NewGitModule() *GitModule {
	return &GitModule{runGit: gitCmd}
}

func (m *GitModule) Name() string { return "git" }

func (m *GitModule) Collect(ctx *Context) error {
	// Fast path: reuse branch/dirty when Antigravity CLI already supplies them
	// and the format does not need ahead/behind counts.
	format := m.DefaultFormat()
	if ctx.Config != nil {
		if f := ctx.Config.ModuleConfig(m.Name()).Format; f != "" {
			format = f
		}
	}
	needAheadBehind := strings.Contains(format, "{ahead}") || strings.Contains(format, "{behind}")
	needDirty := strings.Contains(format, "{dirty}")

	var vcsBranch string
	var vcsHasDirty bool
	var vcsDirty bool
	if ctx.Stdin != nil && ctx.Stdin.VCS != nil {
		vcsBranch = strings.TrimSpace(ctx.Stdin.VCS.Branch)
		if ctx.Stdin.VCS.Dirty != nil {
			vcsHasDirty = true
			vcsDirty = *ctx.Stdin.VCS.Dirty
		}
	}

	if vcsBranch != "" && (!needDirty || vcsHasDirty) && !needAheadBehind {
		m.branch = vcsBranch
		m.dirty = vcsDirty
		return nil
	}

	cwd := ctx.CWD
	if cwd == "" {
		if vcsBranch != "" {
			m.branch = vcsBranch
			m.dirty = vcsDirty
			return nil
		}
		return errors.New("no cwd")
	}

	var (
		wg           sync.WaitGroup
		branch       = vcsBranch
		porcelain    string
		revlist      string
		branchErr    error
		porcelainErr error
		revlistErr   error
	)

	if vcsBranch == "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			branch, branchErr = m.runGit(cwd, "rev-parse", "--abbrev-ref", "HEAD")
		}()
	}

	if needDirty && !vcsHasDirty {
		wg.Add(1)
		go func() {
			defer wg.Done()
			porcelain, porcelainErr = m.runGit(cwd, "--no-optional-locks", "status", "--porcelain")
		}()
	}

	if needAheadBehind {
		wg.Add(1)
		go func() {
			defer wg.Done()
			revlist, revlistErr = m.runGit(cwd, "rev-list", "--left-right", "--count", "@{upstream}...HEAD")
		}()
	}

	wg.Wait()

	if branchErr != nil {
		return branchErr
	}

	m.branch = strings.TrimSpace(branch)
	if m.branch == "" {
		return errors.New("empty git branch")
	}

	if vcsHasDirty {
		m.dirty = vcsDirty
	} else if needDirty && porcelainErr == nil {
		m.dirty = parsePorcelainDirty(porcelain)
	}

	if needAheadBehind {
		if revlistErr == nil {
			m.ahead, m.behind = parseAheadBehind(revlist)
		} else {
			debug.Log("git", "rev-list error (no upstream?): %v", revlistErr)
		}
	}

	return nil
}

func (m *GitModule) Vars(ctx *Context) map[string]string {
	if m.branch == "" {
		return nil
	}
	vars := map[string]string{
		"branch": ansi.Colored(ansi.BoldGreen, m.branch),
		"dirty":  "",
		"ahead":  "",
		"behind": "",
	}
	if m.dirty {
		vars["dirty"] = ansi.YellowText(ctx.Config.Icon("dirty"))
	}
	if m.ahead > 0 {
		vars["ahead"] = fmt.Sprintf("↑%d", m.ahead)
	}
	if m.behind > 0 {
		vars["behind"] = fmt.Sprintf("↓%d", m.behind)
	}
	return vars
}

func (m *GitModule) DefaultFormat() string {
	return "{branch}{dirty}"
}

func parsePorcelainDirty(output string) bool {
	return strings.TrimSpace(output) != ""
}

func parseAheadBehind(output string) (ahead, behind int) {
	parts := strings.Fields(strings.TrimSpace(output))
	if len(parts) != 2 {
		return 0, 0
	}
	// git rev-list --left-right --count @{upstream}...HEAD
	// outputs: <upstream_count>\t<head_count> = <behind>\t<ahead>
	behind, _ = strconv.Atoi(parts[0])
	ahead, _ = strconv.Atoi(parts[1])
	return ahead, behind
}

func gitCmd(cwd string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd
	cmd.WaitDelay = 100 * time.Millisecond
	out, err := cmd.Output()
	return string(out), err
}
