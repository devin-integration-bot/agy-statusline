package stdin

import (
	"strings"
	"testing"
)

func TestParse_FullPayload(t *testing.T) {
	raw := `{
		"cwd": "/tmp/cwd",
		"workspace": {"current_dir": "/tmp/current", "project_dir": "/tmp/proj"},
		"transcript_path": "/tmp/transcript.jsonl",
		"model": {"id": "gemini-4", "display_name": "Gemini 4 Neon (High)", "effort": "high"},
		"context_window": {
			"context_window_size": 1048576,
			"used_percentage": 14.24,
			"total_input_tokens": 88244,
			"total_output_tokens": 61074,
			"current_usage": {
				"input_tokens": 60000,
				"output_tokens": 500,
				"cache_creation_input_tokens": 2000,
				"cache_read_input_tokens": 20000
			}
		},
		"quota": {
			"gemini-5h": {"remaining_fraction": 0.8, "reset_in_seconds": 3600},
			"gemini-weekly": {"remaining_fraction": 0.9, "reset_time": "2026-11-29T00:00:00Z"}
		},
		"plan_tier": "Pro",
		"subagents": [{"name": "research", "role": "Codebase Researcher", "status": "running"}],
		"vcs": {"type": "git", "branch": "main", "dirty": true},
		"cost": {"total_usd": 0.42, "estimated": true}
	}`

	d, err := Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if got := d.ModelName(); got != "Gemini 4 Neon (High)" {
		t.Errorf("ModelName = %q", got)
	}
	if got := d.ModelEffort(); got != "high" {
		t.Errorf("ModelEffort = %q", got)
	}
	if got := d.EffectiveCWD(); got != "/tmp/current" {
		t.Errorf("EffectiveCWD = %q", got)
	}
	if got := d.TotalTokens(); got != 82000 {
		t.Errorf("TotalTokens = %d, want 82000", got)
	}
	in, cache := d.TokenBreakdown()
	if in != 60000 || cache != 22000 {
		t.Errorf("TokenBreakdown = (%d, %d), want (60000, 22000)", in, cache)
	}
	if len(d.Quota) != 2 {
		t.Errorf("len(Quota) = %d, want 2", len(d.Quota))
	}
	if len(d.Subagents) != 1 || d.Subagents[0].Status != "running" {
		t.Errorf("Subagents = %+v", d.Subagents)
	}
}

func TestParse_MistypedFieldTolerated(t *testing.T) {
	raw := `{"cwd": "/tmp/a", "terminal_width": "not-an-int", "plan_tier": "Pro"}`
	d, err := Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("expected mistyped field to be tolerated, got error: %v", err)
	}
	if d.CWD != "/tmp/a" {
		t.Errorf("CWD = %q, want /tmp/a", d.CWD)
	}
	if d.PlanTier != "Pro" {
		t.Errorf("PlanTier = %q, want Pro", d.PlanTier)
	}
}

func TestParse_MalformedAndNonObjectJSON(t *testing.T) {
	for _, raw := range []string{"{bad json", "[]", `"str"`, "123"} {
		if _, err := Parse(strings.NewReader(raw)); err == nil {
			t.Errorf("expected error for input %s", raw)
		}
	}
}

func TestTotalTokens_Fallbacks(t *testing.T) {
	pct := 25.0
	tests := []struct {
		name string
		data Data
		want int
	}{
		{
			name: "current_usage_preferred",
			data: Data{ContextWindow: ContextWindow{
				Size:           1_000_000,
				UsedPercentage: &pct,
				CurrentUsage:   &TokenUsage{InputTokens: 1000, CacheReadInputTokens: 2000},
			}},
			want: 3000,
		},
		{
			name: "used_percentage_fallback",
			data: Data{ContextWindow: ContextWindow{
				Size:           1_000_000,
				UsedPercentage: &pct,
			}},
			want: 250000,
		},
		{
			name: "total_tokens_fallback",
			data: Data{ContextWindow: ContextWindow{
				TotalInputTokens:  5000,
				TotalOutputTokens: 1000,
			}},
			want: 6000,
		},
		{
			name: "empty",
			data: Data{},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.data.TotalTokens(); got != tt.want {
				t.Errorf("TotalTokens() = %d, want %d", got, tt.want)
			}
		})
	}
}
