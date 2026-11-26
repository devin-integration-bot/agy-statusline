# Architectural Decisions

### ADR-001: Per-session live TPS tracking with transcript fallback (2026-11-26)

**Context:**
- Antigravity CLI stdin JSON does not provide native TPS or per-step duration fields (`duration_ms`).
- `transcript.jsonl` only provides 1-second timestamps and only pairs `PLANNER_RESPONSE -> GENERIC` for tool-calling steps (not pure text replies).
- Concurrent terminal sessions must not overwrite each other's timing state.

**Decision:**
- Implement `tps` as a standalone module positioned after `model` on line 1.
- Track live `agent_state` transitions and `context_window.total_output_tokens` deltas in per-conversation state files under `$AGY_CONFIG_DIR/plugins/agy-statusline/cache/tps/<session_id>.json` using atomic temp-file rename.
- Fall back to `PLANNER_RESPONSE -> GENERIC` step pairs parsed during the existing single-pass `transcript.jsonl` scan when no live measurement exists yet.

**Alternatives Considered:**
- Pure `transcript.jsonl` parsing -> Rejected: 1-second granularity and cannot measure text-only final responses.
- Shared state file under `/tmp` -> Rejected: cross-session contention and world-writable `/tmp` safety risk on multi-user Linux.

**Consequences:**
- Millisecond-precision end-to-end throughput covering both tool-calling and text-only responses.
- Immediate cold-start display via transcript fallback with zero extra JSON unmarshals.
- Measures end-to-end effective output speed (includes TTFT and thinking time) since CLI does not expose TTFT separately.
