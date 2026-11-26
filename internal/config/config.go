package config

import (
	"encoding/json"
	"errors"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/mritd/agy-statusline/internal/debug"
)

type Config struct {
	Modules      []string            `json:"modules"`
	Separator    string              `json:"separator"`
	Newline      []string            `json:"newline"`
	LinePrefixes []string            `json:"line_prefixes,omitempty"`
	Responsive   []ResponsiveLayout  `json:"responsive,omitempty"`
	BarStyle     string              `json:"bar_style"`
	BarStyles    map[string][]string `json:"-"` // merged from "bar_styles" in Load
	Icons        map[string]string   `json:"-"` // merged from "icons" in Load
	MaxTailSize  string              `json:"max_tail_size"`

	moduleConfs map[string]ModuleConf // per-module sections, parsed once in Load
}

// ResponsiveLayout overrides layout settings when stdin terminal_width meets
// or exceeds MinWidth. Disabled by default unless configured under "responsive".
type ResponsiveLayout struct {
	MinWidth     int      `json:"min_width"`
	Modules      []string `json:"modules"`
	Newline      []string `json:"newline"`
	Separator    *string  `json:"separator"`
	LinePrefixes []string `json:"line_prefixes"`
}

// ResolvedLayout holds the effective layout for a single render pass.
type ResolvedLayout struct {
	Modules      []string
	Newline      []string
	Separator    string
	LinePrefixes []string
}

func (l ResolvedLayout) StartsNewline(name string) bool {
	return slices.Contains(l.Newline, name)
}

type ModuleConf struct {
	Format       string `json:"format"`
	BarWidth     int    `json:"bar_width"`
	ContextLimit *int   `json:"context_limit,omitempty"`
}

var defaultBarStyles = map[string][]string{
	"block":      {"█", "░"},
	"square":     {"■", "□"},
	"small":      {"▪", "▫"},
	"bar":        {"▰", "▱"},
	"dot":        {"●", "○"},
	"diamond":    {"◆", "◇"},
	"line":       {"━", "─"},
	"braille":    {"⣿", "⣀"},
	"shade":      {"▓", "░"},
	"half":       {"▄", "▁"},
	"half-block": {"▌", "░"},
	"fine":       {"█", "▓", "▒", "░", "·"},
}

var defaultIcons = map[string]string{
	"running":        "≡",
	"completed":      "✓",
	"error":          "✗",
	"dirty":          "*",
	"state_idle":     "●",
	"state_thinking": "◆",
	"state_working":  "⚙",
	"state_tool_use": "🔧",
	"state_other":    "⏳",
}

// DefaultMaxTailSize keeps transcript scanning fast even on huge sessions
// while covering recent turns.
const DefaultMaxTailSize = "2MB"

const defaultTerminalWidth = 80

func cloneBarStyles(src map[string][]string) map[string][]string {
	dst := make(map[string][]string, len(src))
	for k, v := range src {
		dst[k] = slices.Clone(v)
	}
	return dst
}

func Default() *Config {
	return &Config{
		Modules:     []string{"context", "usage", "model", "tps", "dir", "git", "tools", "agents", "environment", "session"},
		Separator:   " | ",
		Newline:     []string{"tools", "agents", "environment"},
		BarStyle:    "diamond",
		BarStyles:   cloneBarStyles(defaultBarStyles),
		Icons:       maps.Clone(defaultIcons),
		MaxTailSize: DefaultMaxTailSize,
	}
}

// GeminiDir returns the shared Gemini directory: $GEMINI_CONFIG_DIR,
// or ~/.gemini when unset.
func GeminiDir() string {
	if dir := os.Getenv("GEMINI_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".gemini")
}

// SharedConfigDir returns the Antigravity shared customization directory
// (~/.gemini/config).
func SharedConfigDir() string {
	return filepath.Join(GeminiDir(), "config")
}

// AgyDir returns the Antigravity CLI data directory: $AGY_CONFIG_DIR,
// or ~/.gemini/antigravity-cli when unset.
func AgyDir() string {
	if dir := os.Getenv("AGY_CONFIG_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(GeminiDir(), "antigravity-cli")
}

// PluginDir returns the directory holding agy-statusline's configuration.
func PluginDir() string {
	return filepath.Join(AgyDir(), "plugins", "agy-statusline")
}

func DefaultPath() string {
	return filepath.Join(PluginDir(), "config.json")
}

func Load(path string) *Config {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeDefault(path, cfg)
		} else {
			debug.Log("config", "load failed (using defaults): %v", err)
		}
		return cfg
	}

	// Top-level fields overlay the defaults. A mistyped field is skipped and
	// the rest still apply; malformed JSON falls back to defaults.
	if err := json.Unmarshal(data, cfg); err != nil {
		var typeErr *json.UnmarshalTypeError
		if !errors.As(err, &typeErr) {
			debug.Log("config", "parse failed (using defaults): %v", err)
			return Default()
		}
		debug.Log("config", "ignoring mistyped field: %v", err)
	}
	if cfg.MaxTailSize == "" {
		cfg.MaxTailSize = DefaultMaxTailSize
	}

	var raw map[string]json.RawMessage
	_ = json.Unmarshal(data, &raw)

	var styles map[string][]string
	if v, ok := raw["bar_styles"]; ok && json.Unmarshal(v, &styles) == nil {
		for k, chars := range styles {
			if len(chars) >= 2 {
				cfg.BarStyles[k] = slices.Clone(chars)
			}
		}
	}
	var icons map[string]string
	if v, ok := raw["icons"]; ok && json.Unmarshal(v, &icons) == nil {
		maps.Copy(cfg.Icons, icons)
	}

	cfg.moduleConfs = make(map[string]ModuleConf, len(raw))
	for name, v := range raw {
		var mc ModuleConf
		_ = json.Unmarshal(v, &mc)
		cfg.moduleConfs[name] = mc
	}
	return cfg
}

// BarStyleChars returns the character sequence for the active bar style,
// ordered from full to empty (e.g. ["◆", "◇"] or ["█", "▓", "▒", "░", "·"]).
func (c *Config) BarStyleChars() []string {
	if c != nil {
		if chars, ok := c.BarStyles[c.BarStyle]; ok && len(chars) >= 2 {
			return slices.Clone(chars)
		}
	}
	return []string{"█", "░"}
}

// BarChars returns the [filled, empty] character pair for the active bar style.
func (c *Config) BarChars() (string, string) {
	chars := c.BarStyleChars()
	return chars[0], chars[len(chars)-1]
}

// Icon returns the icon for the given key, falling back to the built-in default.
func (c *Config) Icon(key string) string {
	if c != nil {
		if v, ok := c.Icons[key]; ok {
			return v
		}
	}
	if v, ok := defaultIcons[key]; ok {
		return v
	}
	return ""
}

// MaxTailBytes returns the parsed max_tail_size in bytes.
// "0" requests a full scan; malformed values fall back to DefaultMaxTailSize
// so a typo cannot accidentally trigger full-file scans on huge transcripts.
func (c *Config) MaxTailBytes() int64 {
	s := strings.TrimSpace(c.MaxTailSize)
	if s == "0" {
		return 0
	}
	if n := ParseSize(s); n > 0 {
		return n
	}
	return ParseSize(DefaultMaxTailSize)
}

// ParseSize parses a human-readable size string like "2MB", "2M", "512KB", "1GB".
// Pure numeric strings are treated as bytes. Returns 0 for empty or invalid input.
func ParseSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0
	}

	upper := strings.ToUpper(s)
	var suffix string
	var multiplier int64
	switch {
	case strings.HasSuffix(upper, "GB"):
		suffix = s[:len(s)-2]
		multiplier = 1024 * 1024 * 1024
	case strings.HasSuffix(upper, "G"):
		suffix = s[:len(s)-1]
		multiplier = 1024 * 1024 * 1024
	case strings.HasSuffix(upper, "MB"):
		suffix = s[:len(s)-2]
		multiplier = 1024 * 1024
	case strings.HasSuffix(upper, "M"):
		suffix = s[:len(s)-1]
		multiplier = 1024 * 1024
	case strings.HasSuffix(upper, "KB"):
		suffix = s[:len(s)-2]
		multiplier = 1024
	case strings.HasSuffix(upper, "K"):
		suffix = s[:len(s)-1]
		multiplier = 1024
	default:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil || n < 0 {
			return 0
		}
		return n
	}

	n, err := strconv.ParseFloat(strings.TrimSpace(suffix), 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return 0
	}
	bytes := n * float64(multiplier)
	if bytes >= float64(math.MaxInt64) {
		return 0
	}
	return int64(bytes)
}

// ModuleConfig returns the per-module section for name, or the zero value
// when the config file has none.
func (c *Config) ModuleConfig(name string) ModuleConf {
	return c.moduleConfs[name]
}

func (c *Config) IsEnabled(name string) bool {
	return slices.Contains(c.Modules, name)
}

func (c *Config) StartsNewline(name string) bool {
	return slices.Contains(c.Newline, name)
}

// ResolveLayout returns the effective layout for the given terminal width.
// When no responsive rules are configured (the default), the base layout is
// returned unchanged. When terminalWidth <= 0, it defaults to 80 columns.
func (c *Config) ResolveLayout(terminalWidth int) ResolvedLayout {
	if c == nil {
		return ResolvedLayout{}
	}
	layout := ResolvedLayout{
		Modules:      c.Modules,
		Newline:      c.Newline,
		Separator:    c.Separator,
		LinePrefixes: c.LinePrefixes,
	}
	if len(c.Responsive) == 0 {
		return layout
	}

	width := terminalWidth
	if width <= 0 {
		width = defaultTerminalWidth
	}

	var best *ResponsiveLayout
	for i := range c.Responsive {
		r := &c.Responsive[i]
		if r.MinWidth <= width && (best == nil || r.MinWidth > best.MinWidth) {
			best = r
		}
	}
	if best == nil {
		return layout
	}
	if best.Modules != nil {
		layout.Modules = best.Modules
	}
	if best.Newline != nil {
		layout.Newline = best.Newline
	}
	if best.Separator != nil {
		layout.Separator = *best.Separator
	}
	if best.LinePrefixes != nil {
		layout.LinePrefixes = best.LinePrefixes
	}
	return layout
}

func writeDefault(path string, cfg *Config) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		debug.Log("config", "create config dir: %v", err)
		return
	}

	out := map[string]any{
		"modules":   cfg.Modules,
		"separator": cfg.Separator,
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		debug.Log("config", "marshal default config: %v", err)
		return
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		debug.Log("config", "write default config: %v", err)
		return
	}
	debug.Log("config", "created default config: %s", path)
}
