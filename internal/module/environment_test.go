package module

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvironmentModule(t *testing.T) {
	geminiDir := t.TempDir()
	sharedDir := filepath.Join(geminiDir, "config")
	cwd := t.TempDir()

	// Create global files
	mustWrite(t, filepath.Join(geminiDir, "AGENTS.md"), "global")
	mustWrite(t, filepath.Join(sharedDir, "rules", "go.md"), "---\n---")
	mustWrite(t, filepath.Join(sharedDir, "rules", "shell.md"), "---\n---")
	if err := os.MkdirAll(filepath.Join(sharedDir, "skills", "commit"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(sharedDir, "mcp_config.json"), `{
		"mcpServers": {
			"active": {"command": "foo"},
			"off": {"command": "bar", "disabled": true}
		}
	}`)
	mustWrite(t, filepath.Join(sharedDir, "hooks.json"), `{"hooks": {"pre": {}, "post": {}}}`)

	// Create workspace files
	mustWrite(t, filepath.Join(cwd, "GEMINI.md"), "local")
	mustWrite(t, filepath.Join(cwd, ".agents", "rules", "project.md"), "---\n---")
	if err := os.MkdirAll(filepath.Join(cwd, ".agents", "skills", "local-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(cwd, ".agents", "hooks.json"), `{"$schema": "x", "lint": {}}`)

	m := NewEnvironmentModule()
	m.geminiDir = geminiDir
	m.sharedDir = sharedDir

	ctx := &Context{CWD: cwd}
	if err := m.Collect(ctx); err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	vars := m.Vars(ctx)
	if vars["agents_md"] != "2" {
		t.Errorf("agents_md = %q, want 2", vars["agents_md"])
	}
	if vars["rules"] != "3" {
		t.Errorf("rules = %q, want 3", vars["rules"])
	}
	if vars["skills"] != "2" {
		t.Errorf("skills = %q, want 2", vars["skills"])
	}
	if vars["mcps"] != "1" {
		t.Errorf("mcps = %q, want 1 (disabled server excluded)", vars["mcps"])
	}
	if vars["hooks"] != "3" {
		t.Errorf("hooks = %q, want 3", vars["hooks"])
	}
}

func TestEnvironmentModule_GlobCharsAndEmptyHooks(t *testing.T) {
	geminiDir := t.TempDir()
	// Directory name contains glob metacharacters like Next.js route groups.
	cwd := filepath.Join(t.TempDir(), "app", "[variants]")
	mustWrite(t, filepath.Join(cwd, ".agents", "rules", "r1.md"), "ok")
	if err := os.MkdirAll(filepath.Join(cwd, ".agents", "rules", "dir.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(cwd, ".agents", "hooks.json"), `{"$schema":"x","version":1,"hooks":{}}`)

	m := NewEnvironmentModule()
	m.geminiDir = geminiDir

	ctx := &Context{CWD: cwd}
	_ = m.Collect(ctx)
	vars := m.Vars(ctx)
	if vars["rules"] != "1" {
		t.Errorf("rules = %q, want 1 (ignoring .md dir and handling [variants])", vars["rules"])
	}
	if vars["hooks"] != "0" {
		t.Errorf("hooks = %q, want 0 for empty hooks object", vars["hooks"])
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
