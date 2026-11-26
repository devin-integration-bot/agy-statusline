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

### ADR-002: Hybrid transcript-anchored per-step TPS to exclude tool time and multi-tool skew (2026-11-26)

**Context:**
- In live Antigravity CLI sessions, `agent_state` stays `"working"` continuously across multi-step tool loops; pure live timing accumulated minutes of tool execution time into a single step and caused TPS to decay while tools ran.
- On multi-tool `PLANNER_RESPONSE` steps, Antigravity streams each `GENERIC` step as soon as its tool call finishes; pairing `PLANNER_RESPONSE` only with `step_index+1` (the first tool) overestimated TPS by 2x-10x.

**Decision:**
- In `internal/transcript`, measure each tool-calling `PLANNER_RESPONSE` against the maximum first-seen `created_at` across all of its `GENERIC` steps (`step_index` in `[pidx+1, pidx+len(tool_calls)]`), waiting at EOF until the final tool call is emitted, and also record `LatestTextStep` for text-only `PLANNER_RESPONSE` entries.
- In `internal/module/tps.go`, use hybrid transcript+live collection whenever transcript planner steps exist: tool-calling steps come directly from tool-time-free transcript boundaries, while text-only final replies anchor their start time to `LatestTextStep.CreatedAt` (preserving sub-second `WorkingStartMs` on single-step replies) and freeze once completed. Pure live mode remains for stateless/no-transcript runs.

**Consequences:**
- Eliminates multi-step tool execution pollution and multi-tool streaming inflation (real-session peaks drop from 850-1460 tok/s to realistic 110-165 tok/s).
- Self-heals existing session TPS cache files automatically on the next refresh.

### ADR-003: Short-step TTFT guard for primary TPS and optional OLS decode/TTFT variables (2026-11-26)

**Context:**
- On large contexts (~200k tokens), TTFT (prefill + queueing + RTT) is 5-10s while true decode speed is ~120-147 tok/s.
- Most turns finish with a short text reply (100-500 tokens) whose end-to-end rate is 80-90% TTFT (e.g. 156 tokens / 11.3s = 13.8 tok/s), causing primary `{tps}` to plunge right when the user finishes a turn.

**Decision:**
- Keep primary `{tps}` on real observed `tokens / duration`, but prevent short steps (`< 1000` tokens) from overwriting `{tps}` once a substantial (`>= 1000` tokens) step exists for the active model (falling back to the latest step during cold start before any substantial step completes). Isolate/reset history when the active model changes.
- Provide opt-in `{decode}`, `{decode_rate}`, and `{ttft}` variables estimated via OLS (`duration = TTFT + tokens / decode_tps`) across same-model steps, gated on `>= 5` samples, `>= 1500` token spread, `R^2 >= 0.5`, and non-negative TTFT.

**Consequences:**
- Primary `{tps}` stays grounded in real observed measurements without TTFT cliff-drops on short final replies or regression instability.
- Users who also want estimated TTFT-free decode throughput can opt in via `{decode}` / `{ttft}` in `tps.format`.
