package module

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/mritd/agy-statusline/internal/stdin"
)

// UsageModule displays Antigravity CLI 5h/7d quota utilization directly from
// the stdin JSON payload (zero network I/O).
type UsageModule struct {
	barWidth int
	quotaBar BarFunc
	now      func() time.Time
	data     *usageData
}

type usageData struct {
	PlanName      string
	HasFiveHour   bool
	FiveHourUsed  int
	FiveHourReset time.Duration
	HasSevenDay   bool
	SevenDayUsed  int
	SevenDayReset time.Duration
}

func NewUsageModule(barWidth int, quotaBar BarFunc) *UsageModule {
	if barWidth <= 0 {
		barWidth = defaultBarWidth
	}
	return &UsageModule{
		barWidth: barWidth,
		quotaBar: quotaBar,
		now:      time.Now,
	}
}

func (m *UsageModule) Name() string { return "usage" }

func (m *UsageModule) Collect(ctx *Context) error {
	m.data = m.extractUsage(ctx.Stdin)
	return nil
}

func (m *UsageModule) Vars(ctx *Context) map[string]string {
	if m.data == nil {
		return nil
	}

	d := m.data
	vars := map[string]string{
		"plan":     d.PlanName,
		"5h_bar":   "",
		"5h_pct":   "",
		"5h_rem":   "",
		"5h_reset": "",
		"7d_bar":   "",
		"7d_pct":   "",
		"7d_rem":   "",
		"7d_reset": "",
	}

	if d.HasFiveHour {
		vars["5h_pct"] = fmt.Sprintf("%d%%", d.FiveHourUsed)
		vars["5h_rem"] = fmt.Sprintf("%d%%", 100-d.FiveHourUsed)
		if m.quotaBar != nil {
			vars["5h_bar"] = m.quotaBar(d.FiveHourUsed, m.barWidth)
		}
		if d.FiveHourReset > 0 {
			vars["5h_reset"] = " (" + formatResetTime(d.FiveHourReset) + ")"
		}
	}

	if d.HasSevenDay {
		vars["7d_pct"] = fmt.Sprintf("%d%%", d.SevenDayUsed)
		vars["7d_rem"] = fmt.Sprintf("%d%%", 100-d.SevenDayUsed)
		if m.quotaBar != nil {
			vars["7d_bar"] = m.quotaBar(d.SevenDayUsed, m.barWidth)
		}
		if d.SevenDayReset > 0 {
			vars["7d_reset"] = " (" + formatResetTime(d.SevenDayReset) + ")"
		}
	}

	return vars
}

func (m *UsageModule) DefaultFormat() string {
	if m.data == nil {
		return "Usage {5h_bar} {5h_pct}{5h_reset} | {7d_bar} {7d_pct}{7d_reset}"
	}
	switch {
	case m.data.HasFiveHour && m.data.HasSevenDay:
		return "Usage {5h_bar} {5h_pct}{5h_reset} | {7d_bar} {7d_pct}{7d_reset}"
	case m.data.HasFiveHour:
		return "Usage {5h_bar} {5h_pct}{5h_reset}"
	case m.data.HasSevenDay:
		return "Usage {7d_bar} {7d_pct}{7d_reset}"
	default:
		return ""
	}
}

func (m *UsageModule) extractUsage(in *stdin.Data) *usageData {
	if in == nil || len(in.Quota) == 0 {
		return nil
	}

	is3P := isThirdPartyModel(in.Model)
	pref5h := []string{"gemini-5h", "5h"}
	pref7d := []string{"gemini-weekly", "weekly", "7d"}
	if is3P {
		pref5h = []string{"3p-5h", "claude-5h", "5h"}
		pref7d = []string{"3p-weekly", "claude-weekly", "weekly", "7d"}
	}

	b5h := pickQuotaBucket(in.Quota, pref5h, isFiveHourKey)
	b7d := pickQuotaBucket(in.Quota, pref7d, isWeeklyKey)

	used5h, ok5h := bucketUsedPercent(b5h)
	used7d, ok7d := bucketUsedPercent(b7d)
	if !ok5h && !ok7d {
		return nil
	}

	now := m.now()
	d := &usageData{
		PlanName: in.PlanTier,
	}
	if ok5h {
		d.HasFiveHour = true
		d.FiveHourUsed = used5h
		d.FiveHourReset = bucketResetDuration(b5h, now)
	}
	if ok7d {
		d.HasSevenDay = true
		d.SevenDayUsed = used7d
		d.SevenDayReset = bucketResetDuration(b7d, now)
	}
	return d
}

func isThirdPartyModel(model *stdin.Model) bool {
	if model == nil {
		return false
	}
	for _, s := range []string{model.DisplayName, model.ID} {
		lower := strings.ToLower(s)
		if strings.Contains(lower, "claude") ||
			strings.Contains(lower, "gpt") ||
			strings.Contains(lower, "openai") {
			return true
		}
	}
	return false
}

func isFiveHourKey(key string) bool {
	k := strings.ToLower(key)
	return strings.Contains(k, "5h") ||
		strings.Contains(k, "five_hour") ||
		strings.Contains(k, "five-hour") ||
		strings.Contains(k, "5_hour") ||
		strings.Contains(k, "5-hour")
}

func isWeeklyKey(key string) bool {
	k := strings.ToLower(key)
	return strings.Contains(k, "week") ||
		strings.Contains(k, "7d") ||
		strings.Contains(k, "seven_day") ||
		strings.Contains(k, "seven-day")
}

func pickQuotaBucket(q map[string]stdin.QuotaBucket, preferred []string, matchFn func(string) bool) *stdin.QuotaBucket {
	for _, key := range preferred {
		if b, ok := q[key]; ok && !b.Disabled && hasRemaining(b) {
			return &b
		}
	}
	// Deterministic fallback across remaining keys.
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		b := q[k]
		if !b.Disabled && hasRemaining(b) && matchFn(k) {
			return &b
		}
	}
	return nil
}

func hasRemaining(b stdin.QuotaBucket) bool {
	return b.RemainingFraction != nil || b.RemainingPercentage != nil
}

func bucketUsedPercent(b *stdin.QuotaBucket) (int, bool) {
	if b == nil {
		return 0, false
	}
	if b.RemainingFraction != nil {
		return clampPercent((1.0 - *b.RemainingFraction) * 100), true
	}
	if b.RemainingPercentage != nil {
		return clampPercent(100.0 - *b.RemainingPercentage), true
	}
	return 0, false
}

func bucketResetDuration(b *stdin.QuotaBucket, now time.Time) time.Duration {
	if b == nil {
		return 0
	}
	if b.ResetInSeconds != nil && *b.ResetInSeconds > 0 {
		return time.Duration(*b.ResetInSeconds * float64(time.Second))
	}
	if b.ResetTime != "" {
		if t, err := time.Parse(time.RFC3339Nano, b.ResetTime); err == nil {
			if d := t.Sub(now); d > 0 {
				return d
			}
		}
	}
	return 0
}

func formatResetTime(d time.Duration) string {
	if d <= 0 {
		return "0m"
	}
	if d < time.Minute {
		return "<1m"
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}

func clampPercent(v float64) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return int(math.Round(v))
}
