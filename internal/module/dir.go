package module

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mritd/agy-statusline/internal/ansi"
)

const maxGitMetaFileBytes = 4096

// DirModule displays the current working directory name:
//   - Inside a git repository (including subdirectories and linked worktrees):
//     uses the main git repository root directory.
//   - Outside a git repository: uses the current working directory.
//   - If the resolved target is the home directory (~) or exactly one level
//     under home (e.g. ~/tmp), preserves the "~/" prefix; otherwise uses the
//     directory basename.
type DirModule struct {
	homeDir func() (string, error)

	display string
	path    string
	gitRoot string
}

func NewDirModule() *DirModule {
	return &DirModule{homeDir: os.UserHomeDir}
}

func (m *DirModule) Name() string { return "dir" }

func (m *DirModule) Collect(ctx *Context) error {
	if ctx == nil || strings.TrimSpace(ctx.CWD) == "" {
		return nil
	}

	cwd := filepath.Clean(strings.TrimSpace(ctx.CWD))
	if !filepath.IsAbs(cwd) {
		if abs, err := filepath.Abs(cwd); err == nil {
			cwd = abs
		}
	}
	gitRoot := findGitRoot(cwd)

	target := cwd
	if gitRoot != "" {
		target = gitRoot
	}

	home, _ := m.homeDir()

	m.gitRoot = gitRoot
	m.path = shortenHomePath(target, home)
	m.display = formatDirDisplay(target, home)
	return nil
}

func (m *DirModule) Vars(ctx *Context) map[string]string {
	if m.display == "" {
		return nil
	}
	rootName := ""
	if m.gitRoot != "" {
		rootName = filepath.Base(m.gitRoot)
	}
	return map[string]string{
		"dir":  ansi.Colored(ansi.BrightBlue, m.display),
		"name": m.display,
		"path": m.path,
		"root": rootName,
	}
}

func (m *DirModule) DefaultFormat() string {
	return "{dir}"
}

// formatDirDisplay formats target according to home-relative depth:
//   - target == home -> "~"
//   - target is 1 level beneath home (e.g. ~/tmp) -> "~/tmp"
//   - otherwise -> filepath.Base(target)
func formatDirDisplay(target, home string) string {
	target = filepath.Clean(target)
	if home != "" {
		home = filepath.Clean(home)
		if rel, err := filepath.Rel(home, target); err == nil {
			if rel == "." {
				return "~"
			}
			if rel != ".." &&
				!strings.HasPrefix(rel, ".."+string(filepath.Separator)) &&
				!strings.ContainsRune(rel, filepath.Separator) {
				return "~/" + rel
			}
		}
	}
	return filepath.Base(target)
}

func shortenHomePath(target, home string) string {
	target = filepath.Clean(target)
	if home == "" {
		return target
	}
	home = filepath.Clean(home)
	rel, err := filepath.Rel(home, target)
	if err != nil {
		return target
	}
	if rel == "." {
		return "~"
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return target
	}
	return "~/" + filepath.ToSlash(rel)
}

// findGitRoot walks upward from cwd to locate the enclosing git repository root.
// For linked git worktrees (.git file pointing to .../worktrees/<name> with a
// commondir file), it resolves to the main repository root directory.
func findGitRoot(cwd string) string {
	dir := filepath.Clean(cwd)
	for {
		gitPath := filepath.Join(dir, ".git")
		info, err := os.Stat(gitPath)
		if err == nil {
			if info.IsDir() {
				return dir
			}
			if root, ok := resolveGitFileRoot(dir, gitPath); ok {
				return root
			}
			return ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func resolveGitFileRoot(dir, gitFilePath string) (string, bool) {
	content, ok := readSmallRegularFile(gitFilePath, maxGitMetaFileBytes)
	if !ok {
		return "", false
	}
	var gitDir string
	for line := range strings.SplitSeq(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if rest, found := strings.CutPrefix(trimmed, "gitdir:"); found {
			gitDir = strings.TrimSpace(rest)
			break
		}
	}
	if gitDir == "" {
		return "", false
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	gitDir = filepath.Clean(gitDir)

	gitDirInfo, err := os.Stat(gitDir)
	if err != nil || !gitDirInfo.IsDir() {
		return "", false
	}

	if commonContent, ok := readSmallRegularFile(filepath.Join(gitDir, "commondir"), maxGitMetaFileBytes); ok {
		common := strings.TrimSpace(commonContent)
		if common != "" {
			if !filepath.IsAbs(common) {
				common = filepath.Join(gitDir, common)
			}
			common = filepath.Clean(common)
			if filepath.Base(common) == ".git" {
				return filepath.Dir(common), true
			}
		}
	}
	return dir, true
}

func readSmallRegularFile(path string, maxBytes int64) (string, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxBytes {
		return "", false
	}
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil || int64(len(data)) > maxBytes {
		return "", false
	}
	return string(data), true
}
