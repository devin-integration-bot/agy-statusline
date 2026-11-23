package module

import (
	"fmt"
	"math"

	"github.com/mritd/agy-statusline/internal/ansi"
)

const defaultBarWidth = 10

// BarFunc renders a progress bar for a given percentage and width.
type BarFunc func(pct, width int) string

const DefaultContextLimit = 512_000

type ContextModule struct {
	barWidth     int
	barFn        BarFunc
	contextLimit int // effective context size for bar/dot; 0 uses full window
}

func NewContextModule(barWidth, contextLimit int, barFn BarFunc) *ContextModule {
	if barWidth <= 0 {
		barWidth = defaultBarWidth
	}
	if contextLimit < 0 {
		contextLimit = 0
	}
	return &ContextModule{barWidth: barWidth, contextLimit: contextLimit, barFn: barFn}
}

func (m *ContextModule) Name() string { return "context" }

func (m *ContextModule) Collect(ctx *Context) error {
	return nil
}

func (m *ContextModule) Vars(ctx *Context) map[string]string {
	if ctx.Stdin == nil {
		return nil
	}

	barWidth := m.barWidth
	if barWidth <= 0 {
		barWidth = defaultBarWidth
	}

	total := ctx.Stdin.TotalTokens()
	size := ctx.Stdin.ContextWindow.Size

	effectiveSize := size
	if m.contextLimit > 0 {
		if size > 0 {
			effectiveSize = min(size, m.contextLimit)
		} else if total > 0 {
			effectiveSize = m.contextLimit
		}
	}

	pct := 0
	overLimit := false
	switch {
	case effectiveSize > 0:
		pct = max(0, int(math.Round(float64(total)/float64(effectiveSize)*100)))
		if pct > 100 {
			overLimit = m.contextLimit > 0 && (size == 0 || m.contextLimit < size)
			pct = 100
		}
	case ctx.Stdin.ContextWindow.UsedPercentage != nil:
		pct = clampPercent(*ctx.Stdin.ContextWindow.UsedPercentage)
	case ctx.Stdin.ContextWindow.RemainingPercentage != nil:
		pct = clampPercent(100.0 - *ctx.Stdin.ContextWindow.RemainingPercentage)
	}

	remaining := effectiveSize - total
	if remaining < 0 {
		remaining = 0
	}

	barStr := ""
	if m.barFn != nil {
		barStr = m.barFn(pct, barWidth)
	}

	dotColor := ansi.ContextColor(pct)

	pctStr := fmt.Sprintf("%d%%", pct)
	if overLimit {
		pctStr = formatTokens(total)
	}

	vars := map[string]string{
		"dot":       ansi.Colored(dotColor, "●"),
		"bar":       barStr,
		"percent":   pctStr,
		"tokens":    formatTokens(total),
		"window":    formatTokens(size),
		"remaining": formatTokens(remaining),
	}

	if pct >= 85 && ctx.Stdin.ContextWindow.CurrentUsage != nil {
		input, cache := ctx.Stdin.TokenBreakdown()
		vars["breakdown"] = fmt.Sprintf("(in:%s cache:%s)", formatTokens(input), formatTokens(cache))
	} else {
		vars["breakdown"] = ""
	}

	return vars
}

func (m *ContextModule) DefaultFormat() string {
	return "{dot} Ctx {bar} {percent}"
}

func formatTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1000:
		return fmt.Sprintf("%dk", n/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
