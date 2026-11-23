package main

import (
	"io"
	"os"

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
		TranscriptPath: data.TranscriptPath,
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

	return render.Render(ctx, registry, cfg)
}
