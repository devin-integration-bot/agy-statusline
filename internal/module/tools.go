package module

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mritd/agy-statusline/internal/ansi"
	"github.com/mritd/agy-statusline/internal/transcript"
)

type ToolsModule struct {
	tools             []transcript.ToolEntry
	sessionToolNames  []string
	sessionToolCounts map[string]int
}

func NewToolsModule() *ToolsModule  { return &ToolsModule{} }
func (m *ToolsModule) Name() string { return "tools" }

func (m *ToolsModule) Collect(ctx *Context) error {
	td := ctx.LoadTranscript()
	if td == nil {
		return errors.New("no transcript")
	}
	m.tools = td.Tools
	m.sessionToolNames = td.SessionToolNames
	m.sessionToolCounts = td.SessionToolCounts
	return nil
}

func (m *ToolsModule) Vars(ctx *Context) map[string]string {
	var running []transcript.ToolEntry
	compCounts := make(map[string]int)
	errCounts := make(map[string]int)
	for i := range m.tools {
		switch m.tools[i].Status {
		case transcript.StatusRunning:
			running = append(running, m.tools[i])
		case transcript.StatusCompleted:
			compCounts[m.tools[i].Name]++
		case transcript.StatusError:
			errCounts[m.tools[i].Name]++
		}
	}

	// Running tools (last 2, with target details).
	var runParts []string
	start := max(len(running)-2, 0)
	for _, t := range running[start:] {
		label := ctx.Config.Icon("running") + " " + t.Name
		if t.Target != "" {
			label += ": " + t.Target
		}
		runParts = append(runParts, ansi.Colored(ansi.Cyan, label))
	}

	runningNames := make(map[string]bool, len(running))
	for _, t := range running {
		runningNames[t.Name] = true
	}

	compIcon := ctx.Config.Icon("completed")
	errIcon := ctx.Config.Icon("error")
	var compParts, errParts []string
	for _, name := range m.sessionToolNames {
		if runningNames[name] {
			continue
		}
		sc := m.sessionToolCounts[name]
		ec := errCounts[name]
		cc := compCounts[name]
		if ec > 0 {
			errParts = append(errParts, fmt.Sprintf("%s %s",
				ansi.Colored(ansi.Red, errIcon),
				ansi.DimText(fmt.Sprintf("%s ×%d", name, sc))))
		} else if cc > 0 {
			compParts = append(compParts, fmt.Sprintf("%s %s",
				ansi.Colored(ansi.Green, compIcon),
				ansi.DimText(fmt.Sprintf("%s ×%d", name, sc))))
		} else {
			compParts = append(compParts, ansi.DimText(fmt.Sprintf("%s %s ×%d", compIcon, name, sc)))
		}
	}

	if len(runParts) == 0 && len(compParts) == 0 && len(errParts) == 0 {
		return nil
	}

	runStr := strings.Join(runParts, " | ")
	compStr := strings.Join(compParts, " | ")
	errStr := strings.Join(errParts, " | ")

	return map[string]string{
		"running":   runStr,
		"completed": compStr,
		"errors":    errStr,
		"summary":   joinNonEmpty(" | ", runStr, compStr, errStr),
	}
}

func (m *ToolsModule) DefaultFormat() string { return "{summary}" }
