package render

import (
	"math"
	"strings"

	"github.com/mritd/agy-statusline/internal/ansi"
)

func Bar(pct, width int, filled, empty string, colorFn func(int) string) string {
	if width <= 0 {
		return ""
	}
	pct = max(0, min(100, pct))
	f := int(math.Round(float64(pct) / 100 * float64(width)))
	e := width - f
	color := colorFn(pct)
	return color + strings.Repeat(filled, f) + ansi.Reset + ansi.Dim + strings.Repeat(empty, e) + ansi.Reset
}

// StepBar renders a progress bar supporting either a 2-character [filled, empty]
// pair or a multi-step scale [full, partial..., empty] ordered from full to empty
// (e.g. ["█", "▓", "▒", "░", "·"]).
func StepBar(pct, width int, chars []string, colorFn func(int) string) string {
	if width <= 0 {
		return ""
	}
	if len(chars) < 2 {
		return Bar(pct, width, "█", "░", colorFn)
	}
	if len(chars) == 2 {
		return Bar(pct, width, chars[0], chars[1], colorFn)
	}

	pct = max(0, min(100, pct))
	fullChar := chars[0]
	emptyChar := chars[len(chars)-1]
	steps := len(chars) - 1

	total := pct * width
	filledCount := total / 100
	remainder := total % 100

	filledPart := strings.Repeat(fullChar, filledCount)
	emptyCount := width - filledCount
	if filledCount < width {
		if level := remainder * steps / 100; level > 0 {
			filledPart += chars[steps-level]
			emptyCount--
		}
	}

	color := colorFn(pct)
	return color + filledPart + ansi.Reset + ansi.Dim + strings.Repeat(emptyChar, emptyCount) + ansi.Reset
}

// NewContextBar returns a bar function bound to context colors and the given characters.
func NewContextBar(chars ...string) func(pct, width int) string {
	return func(pct, width int) string {
		return StepBar(pct, width, chars, ansi.ContextColor)
	}
}

// NewQuotaBar returns a bar function bound to quota colors and the given characters.
func NewQuotaBar(chars ...string) func(pct, width int) string {
	return func(pct, width int) string {
		return StepBar(pct, width, chars, ansi.QuotaColor)
	}
}
