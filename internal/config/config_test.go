package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_CreatesDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugins", "agy-statusline", "config.json")

	cfg := Load(path)
	if len(cfg.Modules) != 7 {
		t.Fatalf("default modules = %v", cfg.Modules)
	}
	if cfg.BarStyle != "diamond" {
		t.Errorf("BarStyle = %q, want diamond", cfg.BarStyle)
	}
	if cfg.MaxTailSize != DefaultMaxTailSize {
		t.Errorf("MaxTailSize = %q, want %q", cfg.MaxTailSize, DefaultMaxTailSize)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected default config file to be created: %v", err)
	}
}

func TestLoad_CustomOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{
		"modules": ["project", "context", "usage"],
		"separator": " · ",
		"bar_style": "stars",
		"bar_styles": {"stars": ["★", "☆"]},
		"icons": {"dirty": "!"},
		"max_tail_size": "512KB",
		"context": {"bar_width": 8, "context_limit": 0}
	}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := Load(path)
	if cfg.Separator != " · " {
		t.Errorf("Separator = %q", cfg.Separator)
	}
	f, e := cfg.BarChars()
	if f != "★" || e != "☆" {
		t.Errorf("BarChars = (%q, %q), want (★, ☆)", f, e)
	}
	if got := cfg.Icon("dirty"); got != "!" {
		t.Errorf("Icon(dirty) = %q, want !", got)
	}
	if got := cfg.Icon("running"); got != "≡" {
		t.Errorf("Icon(running) = %q, want default ≡", got)
	}
	if got := cfg.MaxTailBytes(); got != 512*1024 {
		t.Errorf("MaxTailBytes = %d, want %d", got, 512*1024)
	}
	mc := cfg.ModuleConfig("context")
	if mc.BarWidth != 8 || mc.ContextLimit == nil || *mc.ContextLimit != 0 {
		t.Errorf("context ModuleConfig = %+v", mc)
	}
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		in   string
		want int64
	}{
		{"", 0},
		{"0", 0},
		{"1024", 1024},
		{"512KB", 512 * 1024},
		{"512K", 512 * 1024},
		{"2MB", 2 * 1024 * 1024},
		{"2M", 2 * 1024 * 1024},
		{"1.5MB", int64(1.5 * 1024 * 1024)},
		{"1GB", 1024 * 1024 * 1024},
		{"-5MB", 0},
		{"NaNMB", 0},
		{"InfMB", 0},
		{"invalid", 0},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := ParseSize(tt.in); got != tt.want {
				t.Errorf("ParseSize(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}

	cfg := Default()
	cfg.MaxTailSize = "not-a-size"
	if got := cfg.MaxTailBytes(); got != 2*1024*1024 {
		t.Errorf("invalid MaxTailSize should fall back to default 2MB, got %d", got)
	}
	cfg.MaxTailSize = "0"
	if got := cfg.MaxTailBytes(); got != 0 {
		t.Errorf("MaxTailSize=0 should return 0 (full scan), got %d", got)
	}
}

func TestEnvDirs(t *testing.T) {
	t.Setenv("GEMINI_CONFIG_DIR", "/custom/gemini")
	t.Setenv("AGY_CONFIG_DIR", "/custom/agy")
	if got := GeminiDir(); got != "/custom/gemini" {
		t.Errorf("GeminiDir = %q", got)
	}
	if got := SharedConfigDir(); got != "/custom/gemini/config" {
		t.Errorf("SharedConfigDir = %q", got)
	}
	if got := AgyDir(); got != "/custom/agy" {
		t.Errorf("AgyDir = %q", got)
	}
	if got := DefaultPath(); got != "/custom/agy/plugins/agy-statusline/config.json" {
		t.Errorf("DefaultPath = %q", got)
	}
}
