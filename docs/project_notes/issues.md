# Work Log

### 2026-11-28 - Add active /goal statusline module after git branch
- **Status**: Verified
- **Description**: Added standalone `goal` module placed after `git`, extracting `/goal` activation from `<ADDITIONAL_METADATA>` and completion/cancellation markers outside CommonMark code spans/fences, rendering `Goal <elapsed> >> <text>` while active with per-session cache across tail-seeked long sessions.
- **Verification**: `go test -race -cover ./...`, `golangci-lint run`, `CGO_ENABLED=0 go install`, adversarial review fixes (all P1/P2/P3), and live `/goal` transcript smoke test all passed.

### 2026-11-26 - Add current directory statusline module before git branch
- **Status**: Verified
- **Description**: Added standalone `dir` module placed before `git`, showing main git repository root (including subdirs and linked worktrees), basename for nested non-git directories, and `~/name` (or `~`) when at most one level under home.
- **Verification**: `go test -race -cover ./...`, `golangci-lint run`, `CGO_ENABLED=0 go install`, adversarial review fixes, and real-config smoke checks all passed.

### 2026-11-26 - Fix TPS accuracy and add short-step guard with optional OLS decode/TTFT
- **Status**: Verified
- **Description**: Switched TPS to hybrid transcript+live per-step measurement using max first-seen `GENERIC` timestamp across all `tool_calls`, anchored text-only replies to `LatestTextStep.CreatedAt`, guarded primary `{tps}` against `< 1000`-token TTFT-dominated steps with per-model reset, and added optional OLS `{decode}` / `{decode_rate}` / `{ttft}` variables.
- **Verification**: `go test -race -cover ./...`, `golangci-lint run`, `CGO_ENABLED=0 go install`, and live-session smoke test all passed.

### 2026-11-26 - Add opt-in parity modules and responsive layout from official statusline.sh
- **Status**: Verified
- **Description**: Added opt-in `state`, `sandbox`, `artifacts`, `subagents`, and `tasks` modules, multi-step `fine` progress bar style, `{percent_exact}` on `context`, count variables on `agents`, and `terminal_width` `responsive` layout with `line_prefixes` (all disabled by default).
- **Verification**: `go test -race -cover ./...`, `golangci-lint run`, and `CGO_ENABLED=0 go build` all passed; verified zero changes to default output.

### 2026-11-26 - Add per-session TPS statusline module
- **Status**: Verified
- **Description**: Added standalone `tps` module placed between `model` and `git`, tracking live output token throughput per conversation with `transcript.jsonl` fallback.
- **Verification**: `go test -race -cover ./...`, `golangci-lint run`, `CGO_ENABLED=0 go build`, and smoke test against live transcript all passed.
- **Notes**: Per-session state is persisted atomically under `$AGY_CONFIG_DIR/plugins/agy-statusline/cache/tps/<session_id>.json` with 7-day stale file pruning.
