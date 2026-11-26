package module

import (
	"strings"

	"github.com/mritd/agy-statusline/internal/ansi"
)

// StateModule displays the current agent_state as a colored status badge
// (e.g. ● READY, ◆ THINKING, ⚙ WORKING, 🔧 TOOL). Disabled by default.
type StateModule struct{}

func NewStateModule() *StateModule                { return &StateModule{} }
func (m *StateModule) Name() string               { return "state" }
func (m *StateModule) Collect(ctx *Context) error { return nil }

func (m *StateModule) Vars(ctx *Context) map[string]string {
	if ctx == nil || ctx.Stdin == nil {
		return nil
	}

	raw := strings.TrimSpace(ctx.Stdin.AgentState)
	if raw == "" {
		raw = "idle"
	}

	var iconKey, label, color string
	switch strings.ToLower(raw) {
	case "idle":
		iconKey = "state_idle"
		label = "READY"
		color = ansi.BoldBrightGreen
	case "thinking":
		iconKey = "state_thinking"
		label = "THINKING"
		color = ansi.BoldBrightYellow
	case "working":
		iconKey = "state_working"
		label = "WORKING"
		color = ansi.BoldBrightCyan
	case "tool_use":
		iconKey = "state_tool_use"
		label = "TOOL"
		color = ansi.BoldBrightMagenta
	default:
		iconKey = "state_other"
		label = strings.ToUpper(raw)
		color = ansi.BoldWhite
	}

	icon := ctx.Config.Icon(iconKey)
	badge := label
	if icon != "" {
		badge = icon + " " + label
	}

	return map[string]string{
		"state": ansi.Colored(color, badge),
		"icon":  icon,
		"label": label,
		"raw":   raw,
	}
}

func (m *StateModule) DefaultFormat() string {
	return "{state}"
}
