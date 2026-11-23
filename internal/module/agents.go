package module

import (
	"errors"
	"fmt"
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

	if len(m.subagents) > 0 {
		for _, sa := range m.subagents {
			name := subagentDisplayName(sa)
			switch strings.ToLower(sa.Status) {
			case "running":
				runParts = append(runParts, ansi.Colored(ansi.Cyan, runIcon+" "+name+"..."))
			case "error", "failed":
				compParts = append(compParts, ansi.Colored(ansi.Red, runIcon+" "+name))
			default:
				compParts = append(compParts, runIcon+" "+name)
			}
		}
	} else {
		for _, a := range m.agents {
			name := agentDisplayName(a)
			switch a.Status {
			case transcript.StatusRunning:
				s := runIcon + " " + name
				if a.Model != "" && a.Model != "inherit" {
					s += fmt.Sprintf(" [%s]", a.Model)
				}
				runParts = append(runParts, ansi.Colored(ansi.Cyan, s+"..."))
			case transcript.StatusCompleted:
				compParts = append(compParts, formatCompletedAgent(runIcon, name, a))
			case transcript.StatusError:
				s := formatCompletedAgent(runIcon, name, a)
				compParts = append(compParts, ansi.Colored(ansi.Red, s))
			}
		}
	}

	// When agents are running, hide completed; otherwise show only the latest.
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

func (m *AgentsModule) DefaultFormat() string { return "{summary}" }

func formatCompletedAgent(icon, name string, a transcript.AgentEntry) string {
	if !a.EndTime.IsZero() && !a.StartTime.IsZero() {
		elapsed := formatElapsed(a.EndTime.Sub(a.StartTime))
		return fmt.Sprintf("%s %s (%s)", icon, name, elapsed)
	}
	return fmt.Sprintf("%s %s", icon, name)
}

func subagentDisplayName(sa stdin.Subagent) string {
	if sa.Role != "" {
		return truncateRunes(sa.Role, 50)
	}
	if sa.Name != "" {
		return truncateRunes(sa.Name, 50)
	}
	return "Agent"
}

func agentDisplayName(a transcript.AgentEntry) string {
	if a.Description != "" {
		return truncateRunes(a.Description, 50)
	}
	if a.Type != "" {
		return a.Type
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
