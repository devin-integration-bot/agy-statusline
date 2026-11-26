# Work Log

### 2026-11-26 - Add per-session TPS statusline module
- **Status**: Verified
- **Description**: Added standalone `tps` module placed between `model` and `git`, tracking live output token throughput per conversation with `transcript.jsonl` fallback.
- **Verification**: `go test -race -cover ./...`, `golangci-lint run`, `CGO_ENABLED=0 go build`, and smoke test against live transcript all passed.
- **Notes**: Per-session state is persisted atomically under `$AGY_CONFIG_DIR/plugins/agy-statusline/cache/tps/<session_id>.json` with 7-day stale file pruning.
