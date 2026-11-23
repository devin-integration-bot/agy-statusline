package module

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mritd/agy-statusline/internal/config"
	"github.com/mritd/agy-statusline/internal/debug"
)

type EnvironmentModule struct {
	agentsMD  int
	rules     int
	skills    int
	mcps      int
	hooks     int
	collected bool

	geminiDir string
	sharedDir string
}

func NewEnvironmentModule() *EnvironmentModule { return &EnvironmentModule{} }
func (m *EnvironmentModule) Name() string      { return "environment" }

func (m *EnvironmentModule) Collect(ctx *Context) error {
	if m.collected {
		return nil
	}
	m.collected = true

	geminiDir := m.geminiDir
	if geminiDir == "" {
		geminiDir = config.GeminiDir()
	}
	sharedDir := m.sharedDir
	if sharedDir == "" {
		if m.geminiDir != "" {
			sharedDir = filepath.Join(geminiDir, "config")
		} else {
			sharedDir = config.SharedConfigDir()
		}
	}

	// Standalone instruction files: global (~/.gemini, ~/.gemini/config) + workspace.
	// Deduplicate cleaned paths in case CWD is inside ~/.gemini.
	instructionPaths := []string{
		filepath.Join(geminiDir, "AGENTS.md"),
		filepath.Join(geminiDir, "GEMINI.md"),
		filepath.Join(sharedDir, "AGENTS.md"),
		filepath.Join(sharedDir, "GEMINI.md"),
	}
	if ctx.CWD != "" {
		instructionPaths = append(instructionPaths,
			filepath.Join(ctx.CWD, "AGENTS.md"),
			filepath.Join(ctx.CWD, "GEMINI.md"),
			filepath.Join(ctx.CWD, ".agents", "AGENTS.md"),
			filepath.Join(ctx.CWD, ".agents", "GEMINI.md"),
		)
	}
	seenPaths := make(map[string]bool, len(instructionPaths))
	for _, p := range instructionPaths {
		clean := filepath.Clean(p)
		if seenPaths[clean] {
			continue
		}
		seenPaths[clean] = true
		if info, err := os.Stat(clean); err == nil && !info.IsDir() {
			m.agentsMD++
		}
	}

	// Modular rules (*.md files, avoiding filepath.Glob pitfalls with '[' in paths).
	m.rules += countRuleFiles(filepath.Join(sharedDir, "rules"))
	if ctx.CWD != "" {
		m.rules += countRuleFiles(filepath.Join(ctx.CWD, ".agents", "rules"))
		m.rules += countRuleFiles(filepath.Join(ctx.CWD, ".agent", "rules"))
	}

	// Skills directories (including symlinks to directories).
	m.skills += countSkillDirs(filepath.Join(sharedDir, "skills"))
	if ctx.CWD != "" {
		m.skills += countSkillDirs(filepath.Join(ctx.CWD, ".agents", "skills"))
	}

	// MCP servers from global and workspace mcp_config.json.
	m.mcps += countMCPConfig(filepath.Join(sharedDir, "mcp_config.json"))
	if ctx.CWD != "" {
		m.mcps += countMCPConfig(filepath.Join(ctx.CWD, ".agents", "mcp_config.json"))
	}

	// Lifecycle hooks from global and workspace hooks.json.
	m.hooks += countHooksConfig(filepath.Join(sharedDir, "hooks.json"))
	if ctx.CWD != "" {
		m.hooks += countHooksConfig(filepath.Join(ctx.CWD, ".agents", "hooks.json"))
	}

	return nil
}

func (m *EnvironmentModule) Vars(ctx *Context) map[string]string {
	return map[string]string{
		"agents_md": fmt.Sprintf("%d", m.agentsMD),
		"rules":     fmt.Sprintf("%d", m.rules),
		"skills":    fmt.Sprintf("%d", m.skills),
		"mcps":      fmt.Sprintf("%d", m.mcps),
		"hooks":     fmt.Sprintf("%d", m.hooks),
	}
}

func (m *EnvironmentModule) DefaultFormat() string {
	return "{agents_md} AGENTS.md | {rules} rules | {skills} skills | {mcps} MCPs | {hooks} hooks"
}

func countRuleFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			continue
		}
		if info, err := os.Stat(filepath.Join(dir, entry.Name())); err == nil && !info.IsDir() {
			count++
		}
	}
	return count
}

func countSkillDirs(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			count++
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if info, err := os.Stat(filepath.Join(dir, entry.Name())); err == nil && info.IsDir() {
				count++
			}
		}
	}
	return count
}

func countMCPConfig(path string) int {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return 0
	}
	var cfg struct {
		MCPServers map[string]struct {
			Disabled bool `json:"disabled"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		debug.Log("environment", "parse mcp config %s: %v", path, err)
		return 0
	}
	count := 0
	for _, srv := range cfg.MCPServers {
		if !srv.Disabled {
			count++
		}
	}
	return count
}

func countHooksConfig(path string) int {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return 0
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		debug.Log("environment", "parse hooks config %s: %v", path, err)
		return 0
	}
	// If an explicit "hooks" section exists, count only its entries (including 0 when empty).
	if rawHooks, ok := top["hooks"]; ok {
		var hooks map[string]json.RawMessage
		if err := json.Unmarshal(rawHooks, &hooks); err == nil {
			return len(hooks)
		}
		return 0
	}
	delete(top, "$schema")
	return len(top)
}
