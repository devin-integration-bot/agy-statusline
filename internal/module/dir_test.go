package module

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mritd/agy-statusline/internal/ansi"
)

func TestDirModule_DisplayRules(t *testing.T) {
	home := t.TempDir()

	// 1. Deep git repo: ~/gopath/src/github.com/mritd/agy-statusline
	repoRoot := filepath.Join(home, "gopath", "src", "github.com", "mritd", "agy-statusline")
	repoSub := filepath.Join(repoRoot, "internal", "module")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repoSub, 0o700); err != nil {
		t.Fatal(err)
	}

	// Linked worktree under repoRoot/.worktrees/feat-dir
	wtMeta := filepath.Join(repoRoot, ".git", "worktrees", "feat-dir")
	wtDir := filepath.Join(repoRoot, ".worktrees", "feat-dir", "pkg")
	if err := os.MkdirAll(wtMeta, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(wtDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtMeta, "commondir"), []byte("../..\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".worktrees", "feat-dir", ".git"), []byte("gitdir: "+wtMeta+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// 2. Non-git nested dir: ~/github/test
	nonGitNested := filepath.Join(home, "github", "test")
	if err := os.MkdirAll(nonGitNested, 0o700); err != nil {
		t.Fatal(err)
	}

	// 3. One level under home: ~/tmp (non-git) and ~/dotfiles (git repo 1 level under home)
	oneLevel := filepath.Join(home, "tmp")
	if err := os.MkdirAll(oneLevel, 0o700); err != nil {
		t.Fatal(err)
	}
	homeGitRepo := filepath.Join(home, "dotfiles")
	homeGitSub := filepath.Join(homeGitRepo, "zsh")
	if err := os.MkdirAll(filepath.Join(homeGitRepo, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeGitSub, 0o700); err != nil {
		t.Fatal(err)
	}

	// Outside home directory
	outside := filepath.Join(t.TempDir(), "outside-dir")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		cwd      string
		wantName string
		wantRoot string
		wantPath string
	}{
		{
			name:     "git_repo_root",
			cwd:      repoRoot,
			wantName: "agy-statusline",
			wantRoot: "agy-statusline",
			wantPath: "~/gopath/src/github.com/mritd/agy-statusline",
		},
		{
			name:     "git_repo_subdir",
			cwd:      repoSub,
			wantName: "agy-statusline",
			wantRoot: "agy-statusline",
			wantPath: "~/gopath/src/github.com/mritd/agy-statusline",
		},
		{
			name:     "git_linked_worktree",
			cwd:      wtDir,
			wantName: "agy-statusline",
			wantRoot: "agy-statusline",
			wantPath: "~/gopath/src/github.com/mritd/agy-statusline",
		},
		{
			name:     "non_git_nested",
			cwd:      nonGitNested,
			wantName: "test",
			wantRoot: "",
			wantPath: "~/github/test",
		},
		{
			name:     "one_level_under_home",
			cwd:      oneLevel,
			wantName: "~/tmp",
			wantRoot: "",
			wantPath: "~/tmp",
		},
		{
			name:     "git_repo_one_level_under_home_from_subdir",
			cwd:      homeGitSub,
			wantName: "~/dotfiles",
			wantRoot: "dotfiles",
			wantPath: "~/dotfiles",
		},
		{
			name:     "home_dir_itself",
			cwd:      home,
			wantName: "~",
			wantRoot: "",
			wantPath: "~",
		},
		{
			name:     "outside_home",
			cwd:      outside,
			wantName: "outside-dir",
			wantRoot: "",
			wantPath: outside,
		},
		{
			name:     "filesystem_root",
			cwd:      string(filepath.Separator),
			wantName: string(filepath.Separator),
			wantRoot: "",
			wantPath: string(filepath.Separator),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &DirModule{homeDir: func() (string, error) { return home, nil }}
			ctx := &Context{CWD: tt.cwd}
			if err := m.Collect(ctx); err != nil {
				t.Fatalf("Collect error: %v", err)
			}
			vars := m.Vars(ctx)
			if vars == nil {
				t.Fatalf("expected non-nil vars for %q", tt.cwd)
			}
			if vars["name"] != tt.wantName {
				t.Errorf("name = %q, want %q", vars["name"], tt.wantName)
			}
			if vars["dir"] != ansi.Colored(ansi.BrightBlue, tt.wantName) {
				t.Errorf("dir = %q, want colored %q", vars["dir"], tt.wantName)
			}
			if vars["root"] != tt.wantRoot {
				t.Errorf("root = %q, want %q", vars["root"], tt.wantRoot)
			}
			if vars["path"] != tt.wantPath {
				t.Errorf("path = %q, want %q", vars["path"], tt.wantPath)
			}
		})
	}
}

func TestDirModule_EmptyCWD(t *testing.T) {
	m := NewDirModule()
	ctx := &Context{}
	if err := m.Collect(ctx); err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	if vars := m.Vars(ctx); vars != nil {
		t.Errorf("expected nil vars on empty CWD, got %+v", vars)
	}
}

func TestDirModule_EdgeCases(t *testing.T) {
	rootTmp := t.TempDir()
	home := filepath.Join(rootTmp, "homeuser")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}

	// Main repo for worktree & submodule tests.
	repoRoot := filepath.Join(home, "gopath", "src", "github.com", "mritd", "agy-statusline")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}

	t.Run("parent_of_home_does_not_render_tilde_dotdot", func(t *testing.T) {
		m := &DirModule{homeDir: func() (string, error) { return home, nil }}
		parent := filepath.Dir(home)
		ctx := &Context{CWD: parent}
		_ = m.Collect(ctx)
		vars := m.Vars(ctx)
		if vars == nil || vars["name"] != filepath.Base(parent) || vars["path"] != parent {
			t.Fatalf("parent of home vars = %+v, want name=%q path=%q", vars, filepath.Base(parent), parent)
		}
	})

	t.Run("relative_gitdir_and_external_worktree", func(t *testing.T) {
		wtMeta := filepath.Join(repoRoot, ".git", "worktrees", "ext-wt")
		if err := os.MkdirAll(wtMeta, 0o700); err != nil {
			t.Fatal(err)
		}
		// Absolute commondir with trailing slash must still resolve to repoRoot (not .git).
		if err := os.WriteFile(filepath.Join(wtMeta, "commondir"), []byte(filepath.Join(repoRoot, ".git")+string(filepath.Separator)+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		extWT := filepath.Join(home, "wt-ext")
		if err := os.MkdirAll(extWT, 0o700); err != nil {
			t.Fatal(err)
		}
		relGitDir, err := filepath.Rel(extWT, wtMeta)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(extWT, ".git"), []byte("gitdir: "+relGitDir+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		m := &DirModule{homeDir: func() (string, error) { return home, nil }}
		ctx := &Context{CWD: extWT}
		_ = m.Collect(ctx)
		vars := m.Vars(ctx)
		if vars == nil || vars["name"] != "agy-statusline" || vars["root"] != "agy-statusline" {
			t.Fatalf("external relative-gitdir worktree vars = %+v, want agy-statusline", vars)
		}
	})

	t.Run("bare_repo_worktree_falls_back_to_worktree_dir", func(t *testing.T) {
		bareGit := filepath.Join(home, "repos", "project.git")
		bareWTMeta := filepath.Join(bareGit, "worktrees", "feature")
		if err := os.MkdirAll(bareWTMeta, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bareWTMeta, "commondir"), []byte("../..\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		bareWT := filepath.Join(home, "work", "feature-wt")
		if err := os.MkdirAll(bareWT, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bareWT, ".git"), []byte("gitdir: "+bareWTMeta+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		m := &DirModule{homeDir: func() (string, error) { return home, nil }}
		ctx := &Context{CWD: bareWT}
		_ = m.Collect(ctx)
		vars := m.Vars(ctx)
		if vars == nil || vars["name"] != "feature-wt" || vars["root"] != "feature-wt" {
			t.Fatalf("bare worktree vars = %+v, want feature-wt", vars)
		}
	})

	t.Run("submodule_without_commondir_uses_submodule_root", func(t *testing.T) {
		modGitDir := filepath.Join(repoRoot, ".git", "modules", "submod")
		if err := os.MkdirAll(modGitDir, 0o700); err != nil {
			t.Fatal(err)
		}
		subDir := filepath.Join(repoRoot, "vendor", "submod")
		subInner := filepath.Join(subDir, "src")
		if err := os.MkdirAll(subInner, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(subDir, ".git"), []byte("gitdir: "+modGitDir+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		m := &DirModule{homeDir: func() (string, error) { return home, nil }}
		ctx := &Context{CWD: subInner}
		_ = m.Collect(ctx)
		vars := m.Vars(ctx)
		if vars == nil || vars["name"] != "submod" || vars["root"] != "submod" {
			t.Fatalf("submodule vars = %+v, want submod", vars)
		}
	})

	t.Run("corrupt_and_oversized_git_file_ignored", func(t *testing.T) {
		badParent := filepath.Join(home, "misc", "broken")
		badChild := filepath.Join(badParent, "leaf")
		if err := os.MkdirAll(badChild, 0o700); err != nil {
			t.Fatal(err)
		}
		// Corrupt .git file (no gitdir: line).
		if err := os.WriteFile(filepath.Join(badParent, ".git"), []byte("not-a-gitfile\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		m := &DirModule{homeDir: func() (string, error) { return home, nil }}
		ctx := &Context{CWD: badChild}
		_ = m.Collect(ctx)
		vars := m.Vars(ctx)
		if vars == nil || vars["name"] != "leaf" || vars["root"] != "" {
			t.Fatalf("corrupt .git vars = %+v, want leaf with empty root", vars)
		}

		// Oversized .git file (> maxGitMetaFileBytes).
		oversized := "gitdir: " + strings.Repeat("a", maxGitMetaFileBytes+10) + "\n"
		if err := os.WriteFile(filepath.Join(badParent, ".git"), []byte(oversized), 0o600); err != nil {
			t.Fatal(err)
		}
		m2 := &DirModule{homeDir: func() (string, error) { return home, nil }}
		_ = m2.Collect(ctx)
		vars2 := m2.Vars(ctx)
		if vars2 == nil || vars2["name"] != "leaf" || vars2["root"] != "" {
			t.Fatalf("oversized .git vars = %+v, want leaf with empty root", vars2)
		}
	})

	t.Run("relative_cwd_and_home_dir_error", func(t *testing.T) {
		sub := filepath.Join(repoRoot, "internal", "sub")
		if err := os.MkdirAll(sub, 0o700); err != nil {
			t.Fatal(err)
		}
		t.Chdir(repoRoot)

		m := &DirModule{homeDir: func() (string, error) { return home, nil }}
		ctx := &Context{CWD: filepath.Join("internal", "sub")}
		_ = m.Collect(ctx)
		vars := m.Vars(ctx)
		if vars == nil || vars["name"] != "agy-statusline" || vars["root"] != "agy-statusline" {
			t.Fatalf("relative CWD vars = %+v, want agy-statusline", vars)
		}

		// homeDir error degrades cleanly to basename.
		mNoHome := &DirModule{homeDir: func() (string, error) { return "", errors.New("no home") }}
		oneLevel := filepath.Join(home, "tmp")
		if err := os.MkdirAll(oneLevel, 0o700); err != nil {
			t.Fatal(err)
		}
		ctxNoHome := &Context{CWD: oneLevel}
		_ = mNoHome.Collect(ctxNoHome)
		varsNoHome := mNoHome.Vars(ctxNoHome)
		if varsNoHome == nil || varsNoHome["name"] != "tmp" || varsNoHome["path"] != oneLevel {
			t.Fatalf("homeDir error vars = %+v, want tmp", varsNoHome)
		}
	})
}
