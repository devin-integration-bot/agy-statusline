package ansi

const (
	Reset             = "\x1b[0m"
	Red               = "\x1b[31m"
	Green             = "\x1b[32m"
	BoldGreen         = "\x1b[1;32m"
	Yellow            = "\x1b[33m"
	Magenta           = "\x1b[35m"
	Cyan              = "\x1b[36m"
	White             = "\x1b[37m"
	BoldWhite         = "\x1b[1;37m"
	BoldBrightGreen   = "\x1b[1;92m"
	BoldBrightYellow  = "\x1b[1;93m"
	BrightBlue        = "\x1b[94m"
	BrightMagenta     = "\x1b[95m"
	BoldBrightMagenta = "\x1b[1;95m"
	BoldBrightCyan    = "\x1b[1;96m"
	Dim               = "\x1b[2m"
)

func Colored(color, text string) string {
	return color + text + Reset
}

func DimText(text string) string {
	return Dim + text + Reset
}

func YellowText(text string) string {
	return Yellow + text + Reset
}

func ContextColor(pct int) string {
	switch {
	case pct >= 85:
		return Red
	case pct >= 70:
		return Yellow
	default:
		return Green
	}
}

func QuotaColor(pct int) string {
	switch {
	case pct >= 90:
		return Red
	case pct >= 75:
		return BrightMagenta
	default:
		return BrightBlue
	}
}
