# Metric definitions

## Token efficiency

- Total tokens: `state_*.sqlite` `threads.tokens_used`.
- Breakdown: the final cumulative rollout `event_msg.token_count.info.total_token_usage`.
- Cached input is a subset of input. Cache ratio is `cached_input_tokens / input_tokens * 100` when input is nonzero.
- Uncached input is `max(input_tokens - cached_input_tokens, 0)`.
- Never sum cumulative `token_count` snapshots.
- When state and rollout totals differ, retain the state total and surface the mismatch.

## Tool activity

- Pair calls and outputs using `call_id`, `tool_use_id`, or a source-defined identifier.
- Prefer response items for tool identity and safe argument/output previews.
- Use lifecycle events for timestamps, duration, and status.
- Merge duplicate representations before counting.
- Keep unmatched calls visible as incomplete coverage.

Tool buckets are Shell, Files & patches, Search & web, MCP & apps, Browser & computer use, Planning & interaction, Multi-agent, and Other.

## Session speed

- Read the effective `service_tier` from persisted `thread_settings_applied` snapshots and compatible `session_configured` events.
- `priority` and legacy `fast` are displayed as **Fast**.
- an absent tier in a complete settings snapshot, or explicit `default`, is displayed as **Standard**.
- observing both Fast and Standard in one session is displayed as **Mixed**, with the number of transitions.
- unsupported tiers or sessions without a supported settings event are **Unavailable** rather than guessed.
- Service tier is operational metadata; it does not measure wall-clock latency or model quality.

## Evidence levels

- Observed: directly persisted in a supported local field or event.
- Correlated: joined across supported records by a stable identifier.
- Inferred: a documented deterministic rule; never present it as a fact.
- Unavailable: missing, redacted, unsupported, or not locally persisted.

## Coaching

Emit no more than three suggestions. Require concrete session evidence. Do not judge a session from token volume, cache ratio, or duration alone.

Historical coaching uses prompt-free operational signatures. Clear repeatable work maps to Luna, read-heavy exploration and everyday work map to Terra, and complex open-ended work maps to Sol. These mappings are inferred recommendations from the versioned coaching policy, not persisted Codex facts.

Use `thread_spawn_edges` to attribute spawned children to top-level tasks. Keep spawned, guardian/review, and top-level populations separate. Do not criticize a complex task for lacking subagents because authorization and parallelizability are unavailable without prompt content.
