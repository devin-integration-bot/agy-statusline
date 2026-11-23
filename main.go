package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mritd/agy-statusline/internal/config"
	"github.com/mritd/agy-statusline/internal/module"
	"github.com/mritd/agy-statusline/internal/render"
	"github.com/mritd/agy-statusline/internal/stdin"
)

func main() {
	if out := execute(os.Stdin, config.DefaultPath()); out != "" {
		render.Print(out)
	}
}

func execute(r io.Reader, cfgPath string) string {
	cfg := config.Load(cfgPath)

	data, err := stdin.Parse(r)
	if err != nil {
		return ""
	}

	ctx := &module.Context{
		Stdin:          data,
		TranscriptPath: resolveTranscriptPath(data),
		MaxTailBytes:   cfg.MaxTailBytes(),
		Config:         cfg,
		CWD:            data.EffectiveCWD(),
	}

	filled, empty := cfg.BarChars()
	contextBar := render.NewContextBar(filled, empty)
	quotaBar := render.NewQuotaBar(filled, empty)

	registry := module.NewRegistry()

	mc := cfg.ModuleConfig("context")
	contextLimit := module.DefaultContextLimit
	if mc.ContextLimit != nil {
		contextLimit = *mc.ContextLimit
	}
	registry.Register(module.NewContextModule(mc.BarWidth, contextLimit, contextBar))

	uc := cfg.ModuleConfig("usage")
	registry.Register(module.NewUsageModule(uc.BarWidth, quotaBar))

	registry.Register(module.NewModelModule())
	registry.Register(module.NewGitModule())
	registry.Register(module.NewToolsModule())
	registry.Register(module.NewAgentsModule())
	registry.Register(module.NewProjectModule())
	registry.Register(module.NewEnvironmentModule())
	registry.Register(module.NewSessionModule())

	return render.Render(ctx, registry, cfg)
}

// resolveTranscriptPath prefers the stdin transcript_path when it exists on disk,
// falling back to $AGY_CONFIG_DIR/brain/<conversation_id>/.system_generated/logs/transcript.jsonl
// when Antigravity CLI omits or passes a non-existent path (e.g. ~/.gemini/antigravity/brain/...).
func resolveTranscriptPath(data *stdin.Data) string {
	if data == nil {
		return ""
	}
	if p := strings.TrimSpace(data.TranscriptPath); p != "" {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}

	id := strings.TrimSpace(data.ConversationID)
	if id == "" {
		id = strings.TrimSpace(data.SessionID)
	}
	if id != "" && !strings.ContainsAny(id, `/\`) && id != "." && id != ".." {
		candidate := filepath.Join(config.AgyDir(), "brain", id, ".system_generated", "logs", "transcript.jsonl")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return data.TranscriptPath
}
