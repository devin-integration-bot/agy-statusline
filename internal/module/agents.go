package module

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/mritd/agy-statusline/internal/ansi"
	"github.com/mritd/agy-statusline/internal/stdin"
	"github.com/mritd/agy-statusline/internal/transcript"
)

type AgentsModule struct {
	subagents []stdin.Subagent
	agents    []transcript.AgentEntry
}

func NewAgentsModule() *AgentsModule { return &AgentsModule{} }
func (m *AgentsModule) Name() string { return "agents" }

func (m *AgentsModule) Collect(ctx *Context) error {
	// Prefer live subagents reported directly on stdin by Antigravity CLI.
	if ctx.Stdin != nil && len(ctx.Stdin.Subagents) > 0 {
		m.subagents = ctx.Stdin.Subagents
		return nil
	}
	td := ctx.LoadTranscript()
	if td == nil {
		return errors.New("no agents data")
	}
	m.agents = td.Agents
	return nil
}

func (m *AgentsModule) Vars(ctx *Context) map[string]string {
	var runParts, compParts []string
	runIcon := ctx.Config.Icon("running")
	compIcon := ctx.Config.Icon("completed")
	errIcon := ctx.Config.Icon("error")

	if len(m.subagents) > 0 {
		for _, sa := range m.subagents {
			name := subagentDisplayName(sa)
			switch strings.ToLower(sa.Status) {
			case "running":
				runParts = append(runParts, ansi.Colored(ansi.Cyan, appendEllipsis(runIcon+" "+name)))
			case "error", "failed":
				compParts = append(compParts, ansi.Colored(ansi.Red, errIcon+" "+name))
			default:
				compParts = append(compParts, compIcon+" "+name)
			}
		}
		// Antigravity CLI's stdin subagents slice has non-deterministic order
		// across refreshes; sort stably so output does not jump between ticks.
		slices.Sort(runParts)
		slices.Sort(compParts)
	} else {
		for _, a := range m.agents {
			name := agentDisplayName(a)
			switch a.Status {
			case transcript.StatusRunning:
				s := runIcon + " " + name
				if a.Model != "" && a.Model != "inherit" {
					s += fmt.Sprintf(" [%s]", a.Model)
				}
				runParts = append(runParts, ansi.Colored(ansi.Cyan, appendEllipsis(s)))
			case transcript.StatusCompleted:
				compParts = append(compParts, formatCompletedAgent(compIcon, name, a))
			case transcript.StatusError:
				s := formatCompletedAgent(errIcon, name, a)
				compParts = append(compParts, ansi.Colored(ansi.Red, s))
			}
		}
	}

	// When agents are running, hide completed; otherwise keep only the latest.
	if len(runParts) > 0 {
		compParts = nil
	} else if len(compParts) > 1 {
		compParts = compParts[len(compParts)-1:]
	}

	if len(runParts) == 0 && len(compParts) == 0 {
		return nil
	}

	runStr := strings.Join(runParts, " | ")
	compStr := strings.Join(compParts, " | ")

	return map[string]string{
		"running":   runStr,
		"completed": compStr,
		"summary":   joinNonEmpty(" | ", runStr, compStr),
	}
}

func (m *AgentsModule) DefaultFormat() string { return "{running}" }

func formatCompletedAgent(icon, name string, a transcript.AgentEntry) string {
	if !a.EndTime.IsZero() && !a.StartTime.IsZero() {
		elapsed := formatElapsed(a.EndTime.Sub(a.StartTime))
		return fmt.Sprintf("%s %s (%s)", icon, name, elapsed)
	}
	return fmt.Sprintf("%s %s", icon, name)
}

func subagentDisplayName(sa stdin.Subagent) string {
	if s := sanitizeSingleLine(sa.Role, 50); s != "" {
		return s
	}
	if s := sanitizeSingleLine(sa.Name, 50); s != "" {
		return s
	}
	return "Agent"
}

func agentDisplayName(a transcript.AgentEntry) string {
	if s := sanitizeSingleLine(a.Description, 50); s != "" {
		return s
	}
	if s := sanitizeSingleLine(a.Type, 50); s != "" {
		return s
	}
	return "Agent"
}

func truncateRunes(s string, maxLen int) string {
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen]) + "..."
}

func appendEllipsis(s string) string {
	if strings.HasSuffix(s, "...") {
		return s
	}
	return s + "..."
}

func formatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Second:
		return "<1s"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	default:
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm %ds", mins, secs)
	}
}
