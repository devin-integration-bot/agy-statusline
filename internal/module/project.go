package module

import (
	"fmt"
	"path/filepath"
)

type ProjectModule struct{}

func NewProjectModule() *ProjectModule              { return &ProjectModule{} }
func (m *ProjectModule) Name() string               { return "project" }
func (m *ProjectModule) Collect(ctx *Context) error { return nil }

func (m *ProjectModule) Vars(ctx *Context) map[string]string {
	model := ""
	effort := ""
	state := ""
	mode := ""
	sandbox := ""
	cost := ""
	tasks := ""
	artifacts := ""

	if ctx.Stdin != nil {
		model = ctx.Stdin.ModelName()
		effort = ctx.Stdin.ModelEffort()
		state = ctx.Stdin.AgentState
		mode = ctx.Stdin.CycleMode
		if ctx.Stdin.Sandbox != nil && ctx.Stdin.Sandbox.Enabled {
			sandbox = "sandbox"
		}
		if ctx.Stdin.Cost != nil && ctx.Stdin.Cost.TotalUSD > 0 {
			cost = fmt.Sprintf("$%.2f", ctx.Stdin.Cost.TotalUSD)
		}
		if ctx.Stdin.TaskCount > 0 {
			tasks = fmt.Sprintf("%d", ctx.Stdin.TaskCount)
		}
		if ctx.Stdin.ArtifactCount > 0 {
			artifacts = fmt.Sprintf("%d", ctx.Stdin.ArtifactCount)
		}
	}

	path := ""
	if ctx.CWD != "" {
		path = filepath.Base(ctx.CWD)
	}

	if model == "" && path == "" {
		return nil
	}
	return map[string]string{
		"model":     model,
		"effort":    effort,
		"path":      path,
		"state":     state,
		"mode":      mode,
		"sandbox":   sandbox,
		"cost":      cost,
		"tasks":     tasks,
		"artifacts": artifacts,
	}
}

func (m *ProjectModule) DefaultFormat() string {
	return "[{model}] {path}"
}
