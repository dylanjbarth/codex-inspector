# Token & Capacity metric contract

All Phase 0 formulas use version `1`. Filters apply to completed-turn
contribution time. Time buckets are calendar buckets in the requested IANA
timezone.

## Catalog

| Key | Unit | Formula v1 |
| --- | --- | --- |
| `recorded_tokens` | tokens | sum normalized `total_tokens` for eligible completed turns |
| `recorded_tokens_by_kind` | tokens | `recorded_tokens` grouped into `user_root_direct`, `descendant`, `inspector_review`, `other_orphan` |
| `recorded_tokens_over_time` | tokens | completion-time calendar buckets grouped by contribution kind |
| `recorded_tokens_by_model_reasoning_over_time` | tokens | completion-time calendar buckets grouped by completed-turn model and reasoning effort |
| `token_composition` | tokens | mutually exclusive `uncached_input`, `cached_input`, `visible_output`, `reasoning_output`, `residual` |
| `top_root_sessions_by_tokens` | tokens | roots ranked by inclusive tokens, returning direct and descendant separately |
| `latest_capacity_observation` | percent | latest source-recorded observation per limit/window identity by observation time, returned in deterministic limit/window order |
| `capacity_drawdown` | percent | ordered source observations partitioned by limit/window and reset boundary |

Both time-series token metrics expose three additive bases per bucket:
`uncached = total_tokens - cached_input_tokens`, `cached = cached_input_tokens`,
and `total = total_tokens`. Missing cache components reduce the individual
metric's coverage rather than being treated as zero.

The catalog response is pinned to an applied index revision. It includes
bounded project, model, reasoning-effort, and contribution-kind choices that
exist at that revision. If the complete choice set exceeds the generated
response bounds, the server returns `413` rather than presenting a silently
truncated filter list.

## Usage normalization

For each session, normalize cumulative snapshots in source order before
applying completion eligibility or query filters. Provisional, aborted, and
otherwise excluded turns contribute no metric value, but their cumulative
snapshot establishes the baseline for the next snapshot. Then, for each
completed turn:

1. Prefer the final source-provided `last_token_usage` associated with that
   turn.
2. Otherwise subtract the preceding cumulative snapshot in the same logical
   session from the final cumulative snapshot. Every component delta must be
   non-negative; otherwise usage is unavailable and a coverage reason is
   recorded.
3. Never add cumulative snapshots.
4. Deduplicate mirrored token records by normalized event identity.

In the supported contract, input includes cached input and output includes
reasoning output. Therefore:

```text
uncached_input = input_tokens - cached_input_tokens
visible_output = output_tokens - reasoning_output_tokens
residual = total_tokens - (uncached_input + cached_input + visible_output + reasoning_output)
```

Negative subtraction makes composition unavailable. A non-zero residual is
shown rather than silently redistributed.

## Filters and attribution

- Time filters compare turn completion timestamps.
- Model/reasoning filters use the completed turn configuration.
- Project uses the root work unit's canonical project.
- Contribution kinds never change labels when filtered.
- Additive responses preserve combined, direct, descendant, Review, and
  other/orphan values where those populations exist.
- Capacity ignores project, model, reasoning, and contribution filters. The
  latest cards also ignore the selected historical time range; the drawdown
  series respects it. Every meaningful limit/window identity is represented by
  its newest complete point; equal observation times use stable source identity
  as a tie-breaker. Drawdown series are separately partitioned by limit, window,
  and reset boundary, so different windows and reset periods are never joined.

Missing or partial fields produce coverage metadata, not zero values. In
particular, if eligible completed turns exist but none has usable recorded
usage, `recorded_tokens` is `null` with `unavailable` coverage. A known sum from
only some eligible turns remains numeric with `derived` coverage and an
explicit gap; it is never labeled exact.

## Golden example

The fake root/descendant fixture yields:

| Result | Expected |
| --- | ---: |
| combined `recorded_tokens` | 2,500 |
| `user_root_direct` | 2,000 |
| `descendant` | 500 |
| `inspector_review` | 0 |
| `other_orphan` | 0 |
| `gpt-fake` / `high` | 2,000 |
| `gpt-fake` / `medium` | 500 |
| uncached input | 1,350 |
| cached input | 600 |
| visible output | 440 |
| reasoning output | 110 |
| residual | 0 |
| top root inclusive/direct/descendant | 2,500 / 2,000 / 500 |
| latest capacity used | 45% |
| UTC day bucket, direct / descendant | 2,000 / 500 |
| capacity drawdown points / reset partitions | 2 / 2 |
| completed-turn usage coverage | 3 / 3 |

The second root turn intentionally omits last-turn usage and contributes an
800-token non-negative delta from cumulative snapshots. Golden expected values
are machine-readable in
[`expected-metrics.json`](../../fixtures/synthetic/expected-metrics.json).
Executable tests also freeze Chicago DST calendar boundaries, missing-usage
coverage, filter consistency, and the rule that project/model/reasoning/kind
filters do not alter capacity while the time range constrains drawdown but not
the latest-observation card.
