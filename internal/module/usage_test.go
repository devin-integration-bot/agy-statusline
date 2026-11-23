package module

import (
	"fmt"
	"testing"
	"time"

	"github.com/mritd/agy-statusline/internal/stdin"
)

func floatPtr(v float64) *float64 { return &v }

func TestUsageModule_GeminiAnd3P(t *testing.T) {
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	barFn := func(pct, width int) string { return fmt.Sprintf("B(%d)", pct) }

	t.Run("gemini_both_windows", func(t *testing.T) {
		m := NewUsageModule(10, barFn)
		m.now = func() time.Time { return now }
		ctx := &Context{
			Stdin: &stdin.Data{
				Model:    &stdin.Model{DisplayName: "Gemini 4 Neon (High)"},
				PlanTier: "Pro",
				Quota: map[string]stdin.QuotaBucket{
					"gemini-5h":     {RemainingFraction: floatPtr(0.96), ResetInSeconds: floatPtr(2*3600 + 22*60)},
					"gemini-weekly": {RemainingFraction: floatPtr(0.69), ResetTime: now.Add(2*24*time.Hour + 14*time.Hour).Format(time.RFC3339)},
					"3p-5h":         {RemainingFraction: floatPtr(0.50)},
				},
			},
		}
		if err := m.Collect(ctx); err != nil {
			t.Fatal(err)
		}
		vars := m.Vars(ctx)
		if vars["5h_pct"] != "4%" || vars["5h_rem"] != "96%" || vars["5h_reset"] != " (2h 22m)" {
			t.Errorf("5h vars = %+v", vars)
		}
		if vars["7d_pct"] != "31%" || vars["7d_rem"] != "69%" || vars["7d_reset"] != " (2d 14h)" {
			t.Errorf("7d vars = %+v", vars)
		}
		if vars["5h_bar"] != "B(4)" || vars["7d_bar"] != "B(31)" || vars["plan"] != "Pro" {
			t.Errorf("bars/plan = %+v", vars)
		}
	})

	t.Run("3p_model_picks_3p_buckets", func(t *testing.T) {
		m := NewUsageModule(10, barFn)
		m.now = func() time.Time { return now }
		ctx := &Context{
			Stdin: &stdin.Data{
				Model: &stdin.Model{DisplayName: "Claude Opus 4.7"},
				Quota: map[string]stdin.QuotaBucket{
					"gemini-5h": {RemainingFraction: floatPtr(0.99)},
					"3p-5h":     {RemainingFraction: floatPtr(0.25), ResetInSeconds: floatPtr(900)},
					"3p-weekly": {RemainingFraction: floatPtr(0.40), ResetInSeconds: floatPtr(86400)},
				},
			},
		}
		_ = m.Collect(ctx)
		vars := m.Vars(ctx)
		if vars["5h_pct"] != "75%" || vars["7d_pct"] != "60%" {
			t.Errorf("3p vars = %+v", vars)
		}
	})

	t.Run("weekly_only_default_format", func(t *testing.T) {
		m := NewUsageModule(10, barFn)
		ctx := &Context{
			Stdin: &stdin.Data{
				Quota: map[string]stdin.QuotaBucket{
					"gemini-weekly": {RemainingFraction: floatPtr(0.90), ResetInSeconds: floatPtr(86400)},
				},
			},
		}
		_ = m.Collect(ctx)
		if got := m.DefaultFormat(); got != "Usage {7d_bar} {7d_pct}{7d_reset}" {
			t.Errorf("DefaultFormat = %q", got)
		}
	})

	t.Run("empty_quota_returns_nil", func(t *testing.T) {
		m := NewUsageModule(10, barFn)
		ctx := &Context{Stdin: &stdin.Data{}}
		_ = m.Collect(ctx)
		if vars := m.Vars(ctx); vars != nil {
			t.Errorf("expected nil vars, got %+v", vars)
		}
	})
}
