package module

import "strconv"

// ArtifactsModule displays the session artifact count reported on stdin.
// Disabled by default.
type ArtifactsModule struct{}

func NewArtifactsModule() *ArtifactsModule            { return &ArtifactsModule{} }
func (m *ArtifactsModule) Name() string               { return "artifacts" }
func (m *ArtifactsModule) Collect(ctx *Context) error { return nil }
func (m *ArtifactsModule) DefaultFormat() string      { return "artifacts {count}" }

func (m *ArtifactsModule) Vars(ctx *Context) map[string]string {
	if ctx == nil || ctx.Stdin == nil {
		return nil
	}
	count := max(0, ctx.Stdin.ArtifactCount)
	return map[string]string{
		"count": formatCount(count),
		"raw":   strconv.Itoa(count),
	}
}
