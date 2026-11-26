package module

import "github.com/mritd/agy-statusline/internal/ansi"

// SandboxModule displays whether Antigravity CLI sandboxing is enabled
// (sandbox ON / sandbox off). Disabled by default.
type SandboxModule struct{}

func NewSandboxModule() *SandboxModule              { return &SandboxModule{} }
func (m *SandboxModule) Name() string               { return "sandbox" }
func (m *SandboxModule) Collect(ctx *Context) error { return nil }

func (m *SandboxModule) Vars(ctx *Context) map[string]string {
	if ctx == nil || ctx.Stdin == nil {
		return nil
	}

	if ctx.Stdin.Sandbox != nil && ctx.Stdin.Sandbox.Enabled {
		status := ansi.Colored(ansi.BoldBrightGreen, "ON")
		return map[string]string{
			"sandbox": ansi.DimText("sandbox ") + status,
			"status":  status,
			"raw":     "on",
		}
	}

	return map[string]string{
		"sandbox": ansi.DimText("sandbox off"),
		"status":  ansi.DimText("off"),
		"raw":     "off",
	}
}

func (m *SandboxModule) DefaultFormat() string {
	return "{sandbox}"
}
