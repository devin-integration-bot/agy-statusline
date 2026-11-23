package ansi

import "testing"

func TestContextColor(t *testing.T) {
	tests := []struct {
		name string
		pct  int
		want string
	}{
		{"zero", 0, Green},
		{"below_warn", 69, Green},
		{"warn_boundary", 70, Yellow},
		{"below_danger", 84, Yellow},
		{"danger_boundary", 85, Red},
		{"full", 100, Red},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContextColor(tt.pct); got != tt.want {
				t.Errorf("ContextColor(%d) = %q, want %q", tt.pct, got, tt.want)
			}
		})
	}
}

func TestQuotaColor(t *testing.T) {
	tests := []struct {
		name string
		pct  int
		want string
	}{
		{"low", 20, BrightBlue},
		{"below_warn", 74, BrightBlue},
		{"warn_boundary", 75, BrightMagenta},
		{"below_danger", 89, BrightMagenta},
		{"danger_boundary", 90, Red},
		{"full", 100, Red},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := QuotaColor(tt.pct); got != tt.want {
				t.Errorf("QuotaColor(%d) = %q, want %q", tt.pct, got, tt.want)
			}
		})
	}
}

func TestHelpers(t *testing.T) {
	if got := Colored(Cyan, "hi"); got != Cyan+"hi"+Reset {
		t.Errorf("Colored = %q", got)
	}
	if got := DimText("hi"); got != Dim+"hi"+Reset {
		t.Errorf("DimText = %q", got)
	}
	if got := YellowText("*"); got != Yellow+"*"+Reset {
		t.Errorf("YellowText = %q", got)
	}
}
