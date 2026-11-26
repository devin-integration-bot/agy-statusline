package render

import (
	"fmt"
	"strings"
	"sync"

	"github.com/mritd/agy-statusline/internal/ansi"
	"github.com/mritd/agy-statusline/internal/config"
	"github.com/mritd/agy-statusline/internal/debug"
	"github.com/mritd/agy-statusline/internal/module"
)

// Render collects data from enabled modules concurrently and formats them in
// configured order with newline grouping and optional responsive layout rules.
func Render(ctx *module.Context, registry *module.Registry, cfg *config.Config) string {
	terminalWidth := 0
	if ctx != nil && ctx.Stdin != nil {
		terminalWidth = ctx.Stdin.TerminalWidth
	}
	layout := cfg.ResolveLayout(terminalWidth)

	modules := registry.Enabled(layout.Modules)
	if len(modules) == 0 {
		return ""
	}

	// Collect all enabled modules concurrently so git/filesystem checks overlap.
	errs := make([]error, len(modules))
	var wg sync.WaitGroup
	for i, m := range modules {
		wg.Add(1)
		go func(idx int, mod module.Module) {
			defer wg.Done()
			errs[idx] = mod.Collect(ctx)
		}(i, m)
	}
	wg.Wait()

	var lines [][]string
	current := []string{}

	for i, m := range modules {
		if errs[i] != nil {
			debug.Log(m.Name(), "collect error: %v", errs[i])
			continue
		}

		vars := m.Vars(ctx)
		if vars == nil {
			continue
		}

		mc := cfg.ModuleConfig(m.Name())
		format := mc.Format
		if format == "" {
			format = m.DefaultFormat()
		}

		output := module.ExpandFormat(format, vars)
		output = strings.TrimSpace(output)
		if output == "" {
			continue
		}

		if layout.StartsNewline(m.Name()) && len(current) > 0 {
			lines = append(lines, current)
			current = []string{}
		}
		current = append(current, output)
	}

	if len(current) > 0 {
		lines = append(lines, current)
	}

	var result []string
	for i, line := range lines {
		lineStr := strings.Join(line, layout.Separator)
		if i < len(layout.LinePrefixes) && layout.LinePrefixes[i] != "" {
			lineStr = layout.LinePrefixes[i] + lineStr
		}
		result = append(result, lineStr)
	}

	return strings.Join(result, "\n")
}

func Print(output string) {
	for _, line := range strings.Split(output, "\n") {
		_, _ = fmt.Println(ansi.Reset + line)
	}
}
