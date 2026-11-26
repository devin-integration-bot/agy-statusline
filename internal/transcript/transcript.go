package transcript

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/mritd/agy-statusline/internal/debug"
)

const (
	maxSessionTools = 6
	minStepDuration = 100 * time.Millisecond
	maxStepDuration = 10 * time.Minute
)

// Status is the lifecycle state of a tool or agent call.
type Status string

const (
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusError     Status = "error"
)

type Data struct {
	Tools             []ToolEntry
	Agents            []AgentEntry
	SessionToolNames  []string       // up to 6 most recently used tool names across session
	SessionToolCounts map[string]int // cumulative tool call counts across session

	// Output throughput derived from PLANNER_RESPONSE -> GENERIC step pairs.
	LastTPS           float64
	AvgTPS            float64
	LastOutputTokens  int
	LastDuration      time.Duration
	TotalOutputTokens int
	TotalDuration     time.Duration
}

type ToolEntry struct {
	ID, Name, Target   string
	Status             Status
	StartTime, EndTime time.Time
}

type AgentEntry struct {
	ID, Type, Model, Description string
	Status                       Status
	StartTime, EndTime           time.Time
}

type pendingStep struct {
	isAgent bool
	idx     int
}

type plannerSample struct {
	stepIndex    int
	outputTokens int
	createdAt    time.Time
	active       bool
}

var (
	patUserInput       = []byte(`"USER_INPUT"`)
	patPlannerResponse = []byte(`"PLANNER_RESPONSE"`)
	patToolCalls       = []byte(`"tool_calls"`)
	patGeneric         = []byte(`"GENERIC"`)
)

// Parse reads an Antigravity CLI transcript.jsonl file (tail-scanned up to
// maxTailBytes when > 0) and extracts tool calls and subagent invocations.
func Parse(path string, maxTailBytes int64) (*Data, error) {
	data := &Data{}
	if path == "" {
		return data, nil
	}

	f, err := os.Open(path)
	if err != nil {
		debug.Log("transcript", "open failed: %v", err)
		return data, nil
	}
	defer func() { _ = f.Close() }()

	var seeked bool
	if maxTailBytes > 0 {
		if info, err := f.Stat(); err == nil && info.Size() > maxTailBytes {
			debug.Log("transcript", "tail scan: file %d bytes, seeking to last %d", info.Size(), maxTailBytes)
			if _, err := f.Seek(-maxTailBytes, io.SeekEnd); err == nil {
				seeked = true
			}
		}
	}

	sessionToolLast := make(map[string]time.Time)
	sessionToolCounts := make(map[string]int)
	var pending []pendingStep
	stepBound := make(map[int]pendingStep)
	var lastPromptTS time.Time
	var lastPlanner plannerSample

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	// After seeking into the middle of the file, discard the likely partial first line.
	if seeked {
		scanner.Scan()
	}

	for scanner.Scan() {
		line := scanner.Bytes()
		// Fast byte pre-filter to skip SYSTEM_MESSAGE, CHECKPOINT, and
		// text-only PLANNER_RESPONSE lines without JSON unmarshaling.
		isUser := bytes.Contains(line, patUserInput)
		isPlanner := bytes.Contains(line, patPlannerResponse) && bytes.Contains(line, patToolCalls)
		isGeneric := bytes.Contains(line, patGeneric)
		if !isUser && !isPlanner && !isGeneric {
			continue
		}

		var entry jsonEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		switch entry.Type {
		case "USER_INPUT":
			lastPromptTS = entry.CreatedAt
			// Clear previous turn's tools and any unmatched pending FIFO steps
			// so cancelled/aborted tool calls cannot misalign future GENERIC steps.
			data.Tools = nil
			pending = nil
			clear(stepBound)
			lastPlanner = plannerSample{}

		case "PLANNER_RESPONSE":
			if entry.OutputTokens > 0 && !entry.CreatedAt.IsZero() {
				lastPlanner = plannerSample{
					stepIndex:    entry.StepIndex,
					outputTokens: entry.OutputTokens,
					createdAt:    entry.CreatedAt,
					active:       true,
				}
			} else {
				lastPlanner = plannerSample{}
			}
			for i, tc := range entry.ToolCalls {
				if tc.Name == "" {
					continue
				}
				id := fmt.Sprintf("%d:%d", entry.StepIndex, i)
				if isAgentTool(tc.Name) {
					desc := unquoteArg(tc.Args["Role"])
					if desc == "" {
						desc = firstLine(unquoteArg(tc.Args["Prompt"]), 50)
					}
					data.Agents = append(data.Agents, AgentEntry{
						ID:          id,
						Type:        unquoteArg(tc.Args["TypeName"]),
						Model:       unquoteArg(tc.Args["Model"]),
						Description: desc,
						Status:      StatusRunning,
						StartTime:   entry.CreatedAt,
					})
					pending = append(pending, pendingStep{isAgent: true, idx: len(data.Agents) - 1})
					continue
				}

				data.Tools = append(data.Tools, ToolEntry{
					ID:        id,
					Name:      tc.Name,
					Target:    extractTarget(tc),
					Status:    StatusRunning,
					StartTime: entry.CreatedAt,
				})
				pending = append(pending, pendingStep{isAgent: false, idx: len(data.Tools) - 1})
				sessionToolLast[tc.Name] = entry.CreatedAt
				sessionToolCounts[tc.Name]++
			}

		case "GENERIC":
			// Measure planner output throughput from the first immediately following
			// GENERIC step, ignoring late status updates for earlier/async steps and
			// consuming lastPlanner so parallel tool steps are not double-counted.
			if lastPlanner.active {
				earlierStep := entry.StepIndex > 0 && lastPlanner.stepIndex > 0 && entry.StepIndex <= lastPlanner.stepIndex
				if !earlierStep {
					adjacent := entry.StepIndex == lastPlanner.stepIndex+1 || (lastPlanner.stepIndex == 0 && entry.StepIndex == 0)
					if adjacent && !entry.CreatedAt.IsZero() {
						if dt := entry.CreatedAt.Sub(lastPlanner.createdAt); dt >= minStepDuration && dt <= maxStepDuration {
							data.LastOutputTokens = lastPlanner.outputTokens
							data.LastDuration = dt
							data.LastTPS = float64(lastPlanner.outputTokens) / dt.Seconds()
							data.TotalOutputTokens += lastPlanner.outputTokens
							data.TotalDuration += dt
						}
					}
					lastPlanner.active = false
				}
			}

			// Reuse binding if the same step_index receives multiple status updates
			// (e.g., RUNNING followed by DONE/ERROR); otherwise bind the next pending step.
			step, ok := stepBound[entry.StepIndex]
			if !ok {
				if len(pending) == 0 {
					continue
				}
				step = pending[0]
				pending = pending[1:]
				if entry.StepIndex > 0 {
					stepBound[entry.StepIndex] = step
				}
			}

			status := stepStatus(entry.Status, entry.Error)
			if step.isAgent {
				if step.idx < len(data.Agents) {
					data.Agents[step.idx].Status = status
					if status != StatusRunning {
						data.Agents[step.idx].EndTime = entry.CreatedAt
					}
				}
			} else if step.idx < len(data.Tools) {
				data.Tools[step.idx].Status = status
				if status != StatusRunning {
					data.Tools[step.idx].EndTime = entry.CreatedAt
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		debug.Log("transcript", "scan error: %v", err)
	}

	// After tail scan, discard orphan running tools and agents from before the last user prompt.
	if seeked && !lastPromptTS.IsZero() {
		var cleanedTools []ToolEntry
		for _, t := range data.Tools {
			if t.Status == StatusRunning && t.StartTime.Before(lastPromptTS) {
				debug.Log("transcript", "discarding orphan running tool %s (%s)", t.Name, t.ID)
				continue
			}
			cleanedTools = append(cleanedTools, t)
		}
		data.Tools = cleanedTools

		var cleanedAgents []AgentEntry
		for _, a := range data.Agents {
			if a.Status == StatusRunning && a.StartTime.Before(lastPromptTS) {
				debug.Log("transcript", "discarding orphan running agent %s (%s)", a.Description, a.ID)
				continue
			}
			cleanedAgents = append(cleanedAgents, a)
		}
		data.Agents = cleanedAgents
	}

	// Build session tool names sorted by most recent usage, limited to 6.
	for name := range sessionToolLast {
		data.SessionToolNames = append(data.SessionToolNames, name)
	}
	slices.SortFunc(data.SessionToolNames, func(a, b string) int {
		ta, tb := sessionToolLast[a], sessionToolLast[b]
		if tb.Before(ta) {
			return -1
		}
		if ta.Before(tb) {
			return 1
		}
		return strings.Compare(a, b)
	})
	if len(data.SessionToolNames) > maxSessionTools {
		data.SessionToolNames = data.SessionToolNames[:maxSessionTools]
	}
	data.SessionToolCounts = sessionToolCounts
	if data.TotalDuration > 0 && data.TotalOutputTokens > 0 {
		data.AvgTPS = float64(data.TotalOutputTokens) / data.TotalDuration.Seconds()
	}

	return data, nil
}

func stepStatus(status, errMsg string) Status {
	if status == "ERROR" || errMsg != "" {
		return StatusError
	}
	if status == "RUNNING" {
		return StatusRunning
	}
	return StatusCompleted
}

func isAgentTool(name string) bool {
	return name == "invoke_subagent"
}

func extractTarget(tc toolCallEntry) string {
	if len(tc.Args) == 0 {
		return ""
	}
	for _, key := range []string{"Path", "FilePath", "file_path", "path"} {
		if v := unquoteArg(tc.Args[key]); v != "" {
			return truncatePath(v, 20)
		}
	}
	for _, key := range []string{"CommandLine", "command", "toolSummary"} {
		if v := unquoteArg(tc.Args[key]); v != "" {
			return firstLine(v, 30)
		}
	}
	for _, key := range []string{"Pattern", "pattern", "query", "Query"} {
		if v := unquoteArg(tc.Args[key]); v != "" {
			return firstLine(v, 20)
		}
	}
	return ""
}

// unquoteArg decodes Antigravity transcript tool_calls args values.
// Antigravity stores each arg value as a JSON-encoded string (often double-quoted
// like "\"/path/to/file\""), so unwrap up to two JSON string layers.
func unquoteArg(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return strings.TrimSpace(string(raw))
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		var inner string
		if err := json.Unmarshal([]byte(s), &inner); err == nil {
			return inner
		}
	}
	return s
}

func firstLine(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\n\r"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func truncatePath(p string, maxLen int) string {
	runes := []rune(p)
	if len(runes) <= maxLen {
		return p
	}
	return "..." + string(runes[len(runes)-maxLen+3:])
}

type jsonEntry struct {
	StepIndex    int             `json:"step_index"`
	Type         string          `json:"type"`
	Status       string          `json:"status"`
	Error        string          `json:"error"`
	CreatedAt    time.Time       `json:"created_at"`
	OutputTokens int             `json:"output_tokens"`
	ToolCalls    []toolCallEntry `json:"tool_calls"`
}

type toolCallEntry struct {
	Name string                     `json:"name"`
	Args map[string]json.RawMessage `json:"args"`
}
