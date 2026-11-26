# Bug Log

### 2026-11-26 - TPS polluted by multi-step tool runtime and inflated on multi-tool responses
- **Issue**: Displayed TPS either decayed severely during multi-step tool loops (single `last_duration_ms` reaching 466s) or spiked to 850-1460 tok/s on multi-tool steps, and dropped to ~13 tok/s after short final text replies.
- **Root Cause**: Antigravity CLI keeps `agent_state="working"` across multi-step tool loops (so pure live tracking accumulated tool execution time), `transcript.go` paired `PLANNER_RESPONSE` only with `step_index+1` instead of the last streamed `GENERIC` tool call, and short final replies (`< 1000` tokens) are dominated by 5-10s TTFT on large contexts.
- **Solution**: Measure tool-calling steps to the max first-seen `created_at` across `[pidx+1, pidx+len(tool_calls)]` (waiting at EOF until all tools arrive), anchor text-only replies to `LatestTextStep.CreatedAt` with `SeenTextStepIndex` cold-start protection, guard primary `{tps}` against `< 1000`-token steps once a substantial step exists, isolate on model switch, and expose optional OLS `{decode}`/`{ttft}`.
- **Prevention**: Regression tests in `internal/transcript/transcript_test.go` and `internal/module/tps_test.go` cover multi-tool streaming, incomplete EOF batches, slow tool loops, cold-start stale text steps, short-step guards, model switches, and OLS guardrails.
