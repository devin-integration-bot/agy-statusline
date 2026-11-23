# agy-statusline

A fast, zero-dependency, modular statusline for [Antigravity CLI (`agy`)](https://antigravity.google/docs/cli/statusline).

Single Go binary. Zero external dependencies. Concurrent data collection. Starts in <5ms.

```
● Ctx ◆◇◇◇◇◇◇◇◇◇ 14% | Usage ◆◇◇◇◇◇◇◇◇◇ 4% (2h 22m) | ◆◆◆◇◇◇◇◇◇◇ 31% (2d 14h) | Gemini 4 Neon (High) | main*
≡ read_file: ...config.go | ✓ run_command ×5 | ✓ edit_file ×3 | ✓ read_file ×12
≡ Review all uncommitted changes...
2 AGENTS.md | 5 rules | 5 skills | 0 MCPs | 0 hooks
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
  "modules": ["context", "usage", "model", "git", "tools", "agents", "environment"],
  "separator": " | "
}
```

### Full configuration reference

```jsonc
{
  // Enabled modules, rendered left-to-right in this order.
  // Available: context, usage, model, git, tools, agents, project, environment
  "modules": ["context", "usage", "model", "git", "tools", "agents", "environment"],

  // Separator between modules on the same line.
  "separator": " | ",

  // Modules listed here start on a new line instead of being separated.
  "newline": ["tools", "agents", "environment"],

  // Progress bar style. Built-in: block, square, small, bar, dot,
  // diamond, line, braille, shade, half, half-block
  "bar_style": "diamond",

  // Custom bar styles. Each entry is [filled, empty] character pair.
  "bar_styles": {
    "stars": ["★", "☆"]
  },

  // Override icons used in the statusline.
  "icons": {
    "running": "≡",
    "completed": "✓",
    "error": "✗",
    "dirty": "*"
  },

  // Max bytes to read from the end of JSONL transcript.
  // Human-readable sizes: "512KB", "2MB", "10MB". Set "0" for full scan.
  "max_tail_size": "2MB",

  // Context window usage.
  // Variables: {dot} {bar} {percent} {tokens} {window} {remaining} {breakdown}
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

  // Git branch info. Variables: {branch} {dirty} {ahead} {behind}
  // Note: git rev-list is only executed when {ahead} or {behind} is used in format.
  "git": {
    "format": "{branch}{dirty}"
  },

  // Tool call tracking. Variables: {running} {completed} {errors} {summary}
  "tools": {
    "format": "{summary}"
  },

  // Subagent tracking. Variables: {running} {completed} {summary}
  "agents": {
    "format": "{summary}"
  },

  // Model and project info (disabled by default).
  // Variables: {model} {effort} {path} {state} {mode} {sandbox} {cost} {tasks} {artifacts}
  "project": {
    "format": "[{model}] {path}"
  },

  // Environment counts. Variables: {agents_md} {rules} {skills} {mcps} {hooks}
  "environment": {
    "format": "{agents_md} AGENTS.md | {rules} rules | {skills} skills | {mcps} MCPs | {hooks} hooks"
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
