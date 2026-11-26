package module

import (
	"strconv"
	"strings"
)

// SubagentsModule displays the subagent count reported on stdin.
// Disabled by default so it can be combined with the running-detail agents module.
type SubagentsModule struct{}

func NewSubagentsModule() *SubagentsModule            { return &SubagentsModule{} }
func (m *SubagentsModule) Name() string               { return "subagents" }
func (m *SubagentsModule) Collect(ctx *Context) error { return nil }
func (m *SubagentsModule) DefaultFormat() string      { return "subagents {count}" }

func (m *SubagentsModule) Vars(ctx *Context) map[string]string {
	if ctx == nil || ctx.Stdin == nil {
		return nil
	}
	total := len(ctx.Stdin.Subagents)
	running := 0
	for _, sa := range ctx.Stdin.Subagents {
		if strings.EqualFold(sa.Status, "running") {
			running++
		}
	}
	return map[string]string{
		"count":       formatCount(total),
		"running":     formatCount(running),
		"raw":         strconv.Itoa(total),
		"raw_running": strconv.Itoa(running),
	}
}
