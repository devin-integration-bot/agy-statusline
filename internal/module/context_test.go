package module

import (
	"strings"
	"testing"

	"github.com/mritd/agy-statusline/internal/stdin"
)

func TestContextModule(t *testing.T) {
	barFn := func(pct, width int) string { return "BAR" }

	t.Run("within_limit", func(t *testing.T) {
		m := NewContextModule(10, DefaultContextLimit, barFn)
		ctx := &Context{
			Stdin: &stdin.Data{
				ContextWindow: stdin.ContextWindow{
					Size: 1_000_000,
					CurrentUsage: &stdin.TokenUsage{
						InputTokens:          51_200,
						CacheReadInputTokens: 51_200,
					},
				},
			},
		}
		vars := m.Vars(ctx)
		if vars["percent"] != "20%" {
			t.Errorf("percent = %q, want 20%%", vars["percent"])
		}
		if vars["tokens"] != "102k" || vars["remaining"] != "409k" || vars["window"] != "1.0M" {
			t.Errorf("vars = %+v", vars)
		}
		if vars["breakdown"] != "" {
			t.Errorf("breakdown should be empty below 85%%, got %q", vars["breakdown"])
		}
	})

	t.Run("over_limit_shows_tokens_and_breakdown", func(t *testing.T) {
		m := NewContextModule(10, DefaultContextLimit, barFn)
		ctx := &Context{
			Stdin: &stdin.Data{
				ContextWindow: stdin.ContextWindow{
					Size: 1_000_000,
					CurrentUsage: &stdin.TokenUsage{
						InputTokens:          200_000,
						CacheReadInputTokens: 400_000,
					},
				},
			},
		}
		vars := m.Vars(ctx)
		if vars["percent"] != "600k" {
			t.Errorf("percent = %q, want 600k", vars["percent"])
		}
		if !strings.Contains(vars["breakdown"], "in:200k") || !strings.Contains(vars["breakdown"], "cache:400k") {
			t.Errorf("breakdown = %q", vars["breakdown"])
		}
	})

	t.Run("zero_limit_uses_full_window", func(t *testing.T) {
		m := NewContextModule(10, 0, barFn)
		pct := 14.2
		ctx := &Context{
			Stdin: &stdin.Data{
				ContextWindow: stdin.ContextWindow{
					Size:           1_000_000,
					UsedPercentage: &pct,
				},
			},
		}
		vars := m.Vars(ctx)
		if vars["percent"] != "14%" {
			t.Errorf("percent = %q, want 14%%", vars["percent"])
		}
		if vars["percent_exact"] != "14.2%" {
			t.Errorf("percent_exact = %q, want 14.2%%", vars["percent_exact"])
		}
	})

	t.Run("small_window_clamps_effective_limit", func(t *testing.T) {
		m := NewContextModule(10, DefaultContextLimit, barFn)
		ctx := &Context{
			Stdin: &stdin.Data{
				ContextWindow: stdin.ContextWindow{
					Size:         128_000,
					CurrentUsage: &stdin.TokenUsage{InputTokens: 128_000},
				},
			},
		}
		vars := m.Vars(ctx)
		if vars["percent"] != "100%" || vars["remaining"] != "0" {
			t.Errorf("small window vars = %+v, want 100%% / 0 remaining", vars)
		}
	})

	t.Run("used_percentage_without_size_or_tokens", func(t *testing.T) {
		m := NewContextModule(10, DefaultContextLimit, barFn)
		pct := 42.5
		ctx := &Context{
			Stdin: &stdin.Data{
				ContextWindow: stdin.ContextWindow{UsedPercentage: &pct},
			},
		}
		vars := m.Vars(ctx)
		if vars["percent"] != "43%" {
			t.Errorf("percent = %q, want 43%%", vars["percent"])
		}
	})
}
