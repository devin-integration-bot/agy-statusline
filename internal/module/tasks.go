package module

import "strconv"

// TasksModule displays the background task count reported on stdin.
// Disabled by default.
type TasksModule struct{}

func NewTasksModule() *TasksModule                { return &TasksModule{} }
func (m *TasksModule) Name() string               { return "tasks" }
func (m *TasksModule) Collect(ctx *Context) error { return nil }
func (m *TasksModule) DefaultFormat() string      { return "tasks {count}" }

func (m *TasksModule) Vars(ctx *Context) map[string]string {
	if ctx == nil || ctx.Stdin == nil {
		return nil
	}
	count := max(0, ctx.Stdin.TaskCount)
	return map[string]string{
		"count": formatCount(count),
		"raw":   strconv.Itoa(count),
	}
}
