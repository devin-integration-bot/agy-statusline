# AGENTS.md

Guidance for coding agents working in `github.com/mritd/agy-statusline`.

## Build & Test Commands

```bash
go build -ldflags="-s -w" -o agy-statusline .
go test -race -cover ./...
golangci-lint run
DEBUG=agy-statusline go run . <<< '{}'
```

Standard Go toolchain only. Zero external dependencies (stdlib only). All platforms build with `CGO_ENABLED=0`.

## Architecture

**Pipeline:** stdin JSON -> parse -> concurrent module Collect -> render formatted lines -> stdout

### Packages under `internal/`

| Package | Responsibility |
|---------|----------------|
| `ansi` | ANSI color constants, helpers (`Colored`, `Dim`, `Yellow`), and threshold color selectors (`ContextColor`, `QuotaColor`) |
| `stdin` | Parses Antigravity CLI statusline JSON payload (model, context_window, quota, subagents, vcs, cost, workspace, transcript_path) |
| `config` | Loads `$AGY_CONFIG_DIR/plugins/agy-statusline/config.json` (default `~/.gemini/antigravity-cli`); resolves `$GEMINI_CONFIG_DIR` (`~/.gemini`) |
| `transcript` | Tail-scans Antigravity `transcript.jsonl` with byte pre-filtering, mapping `PLANNER_RESPONSE.tool_calls` to subsequent `GENERIC` steps |
| `module` | Pluggable module interface (`Name`, `Collect`, `Vars`, `DefaultFormat`) and registry; `Context.LoadTranscript()` deduplicates concurrent parses via `sync.Once` |
| `render` | Concurrent module collection, closure-injected progress bars (`NewContextBar`/`NewQuotaBar`), and multiline formatting |
| `debug` | Conditional stderr logging enabled by `DEBUG=agy-statusline` or `DEBUG=*` |

### Key Conventions

- Keep zero external dependencies (stdlib only).
- Never add blocking network I/O to statusline execution; Antigravity CLI enforces a 5s hard timeout.
- Preserve `CGO_ENABLED=0` compatibility and rune-aware truncation (`[]rune`).
- Keep `Vars()` returning `nil` when a module has nothing to display.
