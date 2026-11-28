# agy-statusline

A fast, zero-dependency, modular statusline for [Antigravity CLI (`agy`)](https://antigravity.google/docs/cli/statusline).

Single Go binary. Zero external dependencies. Concurrent data collection. Starts in <5ms.

```
● Ctx ◆◇◇◇◇◇◇◇◇◇ 14% | Usage ◆◇◇◇◇◇◇◇◇◇ 4% (2h 22m) | ◆◆◆◇◇◇◇◇◇◇ 31% (2d 14h) | Gemini 4 Neon (High) | 85.2 tok/s | agy-statusline | main*
≡ read_file: ...config.go | ✓ run_command ×5 | ✓ edit_file ×3 | ✓ read_file ×12
≡ Review all uncommitted changes...
2 AGENTS.md | 5 rules | 5 skills | 0 MCPs | 0 hooks | 73f8c8da-819a-4dfa-9bd3-575c931cc932
```

## Install

### One-liner

```bash
go install github.com/mritd/agy-statusline@latest
```

Then configure `~/.gemini/antigravity-cli/settings.json` (or run `/statusline agy-statusline` inside `agy`):

```json
{
  "statusLine": {
    "type": "command",
    "command": "agy-statusline",
    "enabled": true
  }
}
```

> Make sure `$(go env GOPATH)/bin` is in your `PATH`, or use the absolute path to `agy-statusline`.

### From source

```bash
git clone https://github.com/mritd/agy-statusline.git
cd agy-statusline
go build -ldflags="-s -w" -o agy-statusline .
mv agy-statusline "$(go env GOPATH)/bin/"
```

## Configuration

A default config is auto-generated on first run at `~/.gemini/antigravity-cli/plugins/agy-statusline/config.json` (or `$AGY_CONFIG_DIR/plugins/agy-statusline/config.json`):

```json
{
  "modules": ["context", "usage", "model", "tps", "dir", "git", "goal", "tools", "agents", "environment", "session"],
  "separator": " | "
}
```

### Full configuration reference

```jsonc
{
  // Enabled modules, rendered left-to-right in this order.
  // Available: context, usage, model, tps, dir, git, goal, tools, agents, project,
  // environment, session, state, sandbox, artifacts, subagents, tasks
  "modules": ["context", "usage", "model", "tps", "dir", "git", "goal", "tools", "agents", "environment", "session"],

  // Separator between modules on the same line.
  "separator": " | ",

  // Modules listed here start on a new line instead of being separated.
  "newline": ["tools", "agents", "environment"],

  // Optional per-line prefixes (disabled by default), e.g. ["╭─ ", "╰─ "].
  // "line_prefixes": ["╭─ ", "╰─ "],

  // Optional terminal_width responsive layout overrides (disabled by default).
  // Matches the highest min_width <= terminal_width (defaults to 80 if omitted).
  // "responsive": [
  //   {
  //     "min_width": 120,
  //     "modules": ["state", "model", "git", "context", "artifacts", "agents", "tasks", "sandbox"],
  //     "newline": [],
  //     "separator": " · "
  //   },
  //   {
  //     "min_width": 80,
  //     "modules": ["state", "model", "git", "context", "artifacts", "agents", "tasks", "sandbox"],
  //     "newline": ["context"],
  //     "separator": " · ",
  //     "line_prefixes": ["╭─ ", "╰─ "]
  //   },
  //   {
  //     "min_width": 0,
  //     "modules": ["state", "model", "context", "tasks"],
  //     "newline": ["context"],
  //     "separator": " · "
  //   }
  // ],

  // Progress bar style. Built-in: block, square, small, bar, dot,
  // diamond, line, braille, shade, half, half-block, fine
  "bar_style": "diamond",

  // Custom bar styles. Each entry is [filled, empty] or a multi-step scale
  // ordered from full to empty (e.g. ["█", "▓", "▒", "░", "·"]).
  "bar_styles": {
    "stars": ["★", "☆"]
  },

  // Override icons used in the statusline.
  "icons": {
    "running": "≡",
    "completed": "✓",
    "error": "✗",
    "dirty": "*",
    "state_idle": "●",
    "state_thinking": "◆",
    "state_working": "⚙",
    "state_tool_use": "🔧",
    "state_other": "⏳"
  },

  // Max bytes to read from the end of JSONL transcript.
  // Human-readable sizes: "512KB", "2MB", "10MB". Set "0" for full scan.
  "max_tail_size": "2MB",

  // Context window usage.
  // Variables: {dot} {bar} {percent} {percent_exact} {tokens} {window} {remaining} {breakdown}
  "context": {
    "format": "{dot} Ctx {bar} {percent}",
    "bar_width": 10,
    "context_limit": 512000
  },

  // Subscription usage quotas (directly from Antigravity CLI stdin quota).
  // Variables: {plan} {5h_bar} {5h_pct} {5h_rem} {5h_reset} {7d_bar} {7d_pct} {7d_rem} {7d_reset}
  "usage": {
    "format": "Usage {5h_bar} {5h_pct}{5h_reset} | {7d_bar} {7d_pct}{7d_reset}",
    "bar_width": 10
  },

  // Active model and reasoning effort. Variables: {model} {name} {id} {effort}
  "model": {
    "format": "{model}"
  },

  // Output token speed (tokens/sec), combining transcript step boundaries with live timing.
  // Variables: {tps} {rate} {avg} {avg_rate} {decode} {decode_rate} {ttft} {tokens} {duration}
  "tps": {
    "format": "{tps}"
  },

  // Current directory (git repo root, basename, or ~/name when 1 level under home).
  // Variables: {dir} {name} {path} {root}
  "dir": {
    "format": "{dir}"
  },

  // Git branch info. Variables: {branch} {dirty} {ahead} {behind}
  // Note: git rev-list is only executed when {ahead} or {behind} is used in format.
  "git": {
    "format": "{branch}{dirty}"
  },

  // Active /goal status, elapsed time, and summary (hidden when completed, cancelled, or inactive).
  // Variables: {goal} {status} {elapsed} {text}
  "goal": {
    "format": "{goal}"
  },

  // Tool call tracking. Variables: {running} {completed} {errors} {summary}
  "tools": {
    "format": "{summary}"
  },

  // Subagent tracking (hidden when none are running by default).
  // Variables: {running} {completed} {summary} {count} {running_count} {completed_count} {raw_count}
  "agents": {
    "format": "{running}"
  },

  // Agent state badge (disabled by default).
  // Variables: {state} {icon} {label} {raw}
  "state": {
    "format": "{state}"
  },

  // Sandbox status badge (disabled by default).
  // Variables: {sandbox} {status} {raw}
  "sandbox": {
    "format": "{sandbox}"
  },

  // Session artifact count (disabled by default).
  // Variables: {count} {raw}
  "artifacts": {
    "format": "artifacts {count}"
  },

  // Subagent count (disabled by default; keeps agents free for running details).
  // Variables: {count} {running} {raw} {raw_running}
  "subagents": {
    "format": "subagents {count}"
  },

  // Background task count (disabled by default).
  // Variables: {count} {raw}
  "tasks": {
    "format": "tasks {count}"
  },

  // Model and project info (disabled by default).
  // Variables: {model} {effort} {path} {state} {mode} {sandbox} {cost} {tasks} {artifacts}
  "project": {
    "format": "[{model}] {path}"
  },

  // Environment counts. Variables: {agents_md} {rules} {skills} {mcps} {hooks}
  "environment": {
    "format": "{agents_md} AGENTS.md | {rules} rules | {skills} skills | {mcps} MCPs | {hooks} hooks"
  },

  // Conversation title (/rename) or fallback UUID.
  // Variables: {session} {title} {id} {short_id}
  "session": {
    "format": "{session}"
  }
}
```

## Performance Design

Antigravity CLI enforces a strict 5-second timeout on statusline commands (at most once every 300ms). `agy-statusline` is designed to stay well below that even under heavy system load:

- **Single static binary**: No shell wrapper, no `jq`, no subprocess chains.
- **Zero network I/O**: Quota and subagent state are consumed directly from stdin JSON.
- **Concurrent collection**: `git`, `transcript`, and `environment` run in parallel; `transcript.jsonl` is parsed at most once via `sync.Once`.
- **Bounded git subprocesses**: 500ms timeout per git command; skips `git rev-list` unless `{ahead}`/`{behind}` is referenced in format; skips git CLI entirely when stdin `vcs` already carries `branch` and `dirty`.
- **Fast transcript scanning**: Reads only the trailing `max_tail_size` (default `2MB`) and byte-filters lines before JSON unmarshaling.

## Debug

```bash
DEBUG=agy-statusline echo '{}' | agy-statusline
```

## License

MIT
