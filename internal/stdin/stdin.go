package stdin

import (
	"encoding/json"
	"errors"
	"io"
	"math"

	"github.com/mritd/agy-statusline/internal/debug"
)

// Data mirrors the JSON state payload Antigravity CLI pipes to statusline commands.
type Data struct {
	CWD               string                 `json:"cwd"`
	SessionID         string                 `json:"session_id"`
	ConversationID    string                 `json:"conversation_id"`
	ConversationTitle string                 `json:"conversation_title"`
	TranscriptPath    string                 `json:"transcript_path"`
	Model             *Model                 `json:"model"`
	Workspace         *Workspace             `json:"workspace"`
	Version           string                 `json:"version"`
	ContextWindow     ContextWindow          `json:"context_window"`
	Exceeds200kTokens *bool                  `json:"exceeds_200k_tokens"`
	Agent             *Agent                 `json:"agent"`
	Product           string                 `json:"product"`
	Quota             map[string]QuotaBucket `json:"quota"`
	AgentState        string                 `json:"agent_state"`
	VCS               *VCS                   `json:"vcs"`
	CycleMode         string                 `json:"cycle_mode"`
	Sandbox           *Sandbox               `json:"sandbox"`
	Subagents         []Subagent             `json:"subagents"`
	ArtifactCount     int                    `json:"artifact_count"`
	PlanTier          string                 `json:"plan_tier"`
	Email             string                 `json:"email"`
	PendingInputCount int                    `json:"pending_input_count"`
	ToolConfirmation  bool                   `json:"tool_confirmation_pending"`
	TaskCount         int                    `json:"task_count"`
	TerminalWidth     int                    `json:"terminal_width"`
	Cost              *Cost                  `json:"cost"`
}

type Model struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Effort      string `json:"effort"`
}

type Workspace struct {
	CurrentDir string `json:"current_dir"`
	ProjectDir string `json:"project_dir"`
}

type ContextWindow struct {
	Size                int         `json:"context_window_size"`
	TotalInputTokens    int         `json:"total_input_tokens"`
	TotalOutputTokens   int         `json:"total_output_tokens"`
	UsedPercentage      *float64    `json:"used_percentage"`
	RemainingPercentage *float64    `json:"remaining_percentage"`
	CurrentUsage        *TokenUsage `json:"current_usage"`
}

type TokenUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

type Agent struct {
	Name string `json:"name"`
}

type QuotaBucket struct {
	RemainingFraction   *float64 `json:"remaining_fraction"`
	RemainingPercentage *float64 `json:"remaining_percentage"`
	RemainingAmount     *float64 `json:"remaining_amount"`
	ResetTime           string   `json:"reset_time"`
	ResetInSeconds      *float64 `json:"reset_in_seconds"`
	Disabled            bool     `json:"disabled"`
}

type VCS struct {
	Type   string `json:"type"`
	Branch string `json:"branch"`
	Dirty  *bool  `json:"dirty"`
}

type Sandbox struct {
	Enabled bool `json:"enabled"`
}

type Subagent struct {
	Name   string `json:"name"`
	Role   string `json:"role"`
	Model  string `json:"model"`
	Status string `json:"status"`
}

type Cost struct {
	TotalUSD    float64 `json:"total_usd"`
	SubagentUSD float64 `json:"subagent_usd"`
	Estimated   bool    `json:"estimated"`
}

// Parse decodes the statusline JSON. A field with an unexpected type is left
// at its zero value so one schema drift cannot blank the whole statusline;
// malformed or non-object JSON is still an error.
func Parse(r io.Reader) (*Data, error) {
	var data Data
	if err := json.NewDecoder(r).Decode(&data); err != nil {
		var typeErr *json.UnmarshalTypeError
		if !errors.As(err, &typeErr) || typeErr.Field == "" {
			return nil, err
		}
		debug.Log("stdin", "ignoring mistyped field %q: %v", typeErr.Field, err)
	}
	return &data, nil
}

// ModelName returns the preferred model display name, falling back to model ID.
func (d *Data) ModelName() string {
	if d == nil || d.Model == nil {
		return ""
	}
	if d.Model.DisplayName != "" {
		return d.Model.DisplayName
	}
	return d.Model.ID
}

// ModelEffort returns the model reasoning effort level if reported.
func (d *Data) ModelEffort() string {
	if d == nil || d.Model == nil {
		return ""
	}
	return d.Model.Effort
}

// EffectiveCWD resolves the best available working directory from workspace or cwd.
func (d *Data) EffectiveCWD() string {
	if d == nil {
		return ""
	}
	if d.Workspace != nil && d.Workspace.CurrentDir != "" {
		return d.Workspace.CurrentDir
	}
	if d.CWD != "" {
		return d.CWD
	}
	if d.Workspace != nil {
		return d.Workspace.ProjectDir
	}
	return ""
}

// TotalTokens returns the current context token usage.
// It prefers current_usage (input + cache tokens), falling back to
// context_window_size * used_percentage (or remaining_percentage),
// then total_input_tokens + total_output_tokens.
func (d *Data) TotalTokens() int {
	if d == nil {
		return 0
	}
	cw := d.ContextWindow
	if cw.CurrentUsage != nil {
		u := cw.CurrentUsage
		if sum := u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens; sum > 0 {
			return sum
		}
	}
	if cw.Size > 0 && cw.UsedPercentage != nil {
		pct := max(0.0, min(100.0, *cw.UsedPercentage))
		return int(math.Round(float64(cw.Size) * pct / 100))
	}
	if cw.Size > 0 && cw.RemainingPercentage != nil {
		pct := max(0.0, min(100.0, 100.0-*cw.RemainingPercentage))
		return int(math.Round(float64(cw.Size) * pct / 100))
	}
	if sum := cw.TotalInputTokens + cw.TotalOutputTokens; sum > 0 {
		return sum
	}
	return 0
}

// TokenBreakdown returns non-cached input tokens and combined cache tokens.
func (d *Data) TokenBreakdown() (input, cache int) {
	if d == nil || d.ContextWindow.CurrentUsage == nil {
		return 0, 0
	}
	u := d.ContextWindow.CurrentUsage
	return u.InputTokens, u.CacheCreationInputTokens + u.CacheReadInputTokens
}
