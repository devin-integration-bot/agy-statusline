package module

import (
	"strings"
	"unicode"
)

type ModelModule struct{}

func NewModelModule() *ModelModule              { return &ModelModule{} }
func (m *ModelModule) Name() string             { return "model" }
func (m *ModelModule) Collect(ctx *Context) error { return nil }

func (m *ModelModule) Vars(ctx *Context) map[string]string {
	if ctx.Stdin == nil {
		return nil
	}
	name := strings.TrimSpace(ctx.Stdin.ModelName())
	if name == "" {
		return nil
	}

	id := ""
	if ctx.Stdin.Model != nil {
		id = strings.TrimSpace(ctx.Stdin.Model.ID)
	}
	effort := formatEffort(ctx.Stdin.ModelEffort())

	full := name
	if effort != "" && !containsEffort(name, effort, ctx.Stdin.ModelEffort()) {
		full = name + " (" + effort + ")"
	}

	return map[string]string{
		"model":  full,
		"name":   name,
		"id":     id,
		"effort": effort,
	}
}

func (m *ModelModule) DefaultFormat() string {
	return "{model}"
}

func formatEffort(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	switch strings.ToLower(s) {
	case "low":
		return "Low"
	case "medium":
		return "Medium"
	case "high":
		return "High"
	case "xhigh":
		return "XHigh"
	case "max":
		return "Max"
	default:
		runes := []rune(s)
		runes[0] = unicode.ToUpper(runes[0])
		return string(runes)
	}
}

func containsEffort(name, formatted, raw string) bool {
	lowerName := strings.ToLower(name)
	for _, candidate := range []string{formatted, raw} {
		c := strings.ToLower(strings.TrimSpace(candidate))
		if c == "" {
			continue
		}
		if strings.Contains(lowerName, "("+c+")") || strings.Contains(lowerName, "["+c+"]") {
			return true
		}
	}
	return false
}
