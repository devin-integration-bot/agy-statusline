package module

import (
	"strings"
	"sync"

	"github.com/mritd/agy-statusline/internal/config"
	"github.com/mritd/agy-statusline/internal/debug"
	"github.com/mritd/agy-statusline/internal/stdin"
	"github.com/mritd/agy-statusline/internal/transcript"
)

type Context struct {
	Stdin          *stdin.Data
	Transcript     *transcript.Data
	TranscriptPath string
	MaxTailBytes   int64
	Config         *config.Config
	CWD            string

	transcriptOnce sync.Once
}

// LoadTranscript parses the transcript at most once across concurrent module
// Collect calls, allowing transcript I/O to overlap with git and filesystem scans.
func (c *Context) LoadTranscript() *transcript.Data {
	if c == nil {
		return nil
	}
	c.transcriptOnce.Do(func() {
		if c.Transcript != nil || c.TranscriptPath == "" {
			return
		}
		td, _ := transcript.Parse(c.TranscriptPath, c.MaxTailBytes)
		c.Transcript = td
	})
	return c.Transcript
}

type Module interface {
	Name() string
	Collect(ctx *Context) error
	Vars(ctx *Context) map[string]string
	DefaultFormat() string
}

type Registry struct {
	modules map[string]Module
}

func NewRegistry() *Registry {
	return &Registry{modules: make(map[string]Module)}
}

func (r *Registry) Register(m Module) {
	r.modules[m.Name()] = m
}

func (r *Registry) Enabled(names []string) []Module {
	var result []Module
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		if m, ok := r.modules[name]; ok {
			seen[name] = true
			result = append(result, m)
		}
	}
	return result
}

func joinNonEmpty(sep string, parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}

func ExpandFormat(format string, vars map[string]string) string {
	var b strings.Builder
	i := 0
	for i < len(format) {
		if format[i] == '}' && i+1 < len(format) && format[i+1] == '}' {
			b.WriteByte('}')
			i += 2
			continue
		}
		if format[i] == '{' {
			if i+1 < len(format) && format[i+1] == '{' {
				b.WriteByte('{')
				i += 2
				continue
			}
			end := strings.IndexByte(format[i+1:], '}')
			if end < 0 {
				b.WriteByte(format[i])
				i++
				continue
			}
			key := format[i+1 : i+1+end]
			if val, ok := vars[key]; ok {
				b.WriteString(val)
			} else {
				debug.Log("format", "unknown var {%s}", key)
			}
			i = i + 1 + end + 1
		} else {
			b.WriteByte(format[i])
			i++
		}
	}
	return b.String()
}
