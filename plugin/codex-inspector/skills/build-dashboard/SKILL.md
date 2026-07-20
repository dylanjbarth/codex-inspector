---
name: build-dashboard
description: Open, refresh, configure, and QA the source-backed local Codex Inspector dashboard. Use when a user asks for a Codex usage dashboard, token or capacity trends, session comparisons, dashboard filters, or a monitoring view of indexed Codex activity.
---

# Build Dashboard

Use Inspector's embedded dashboard as the calculation and rendering surface. Do not recreate its metrics from raw Codex files.

## Workflow

1. Run `codex-inspector doctor --json`. If the CLI is missing, invoke `$codex-inspector:setup`; stop on an unhealthy result.
2. Run `codex-inspector status --json` to establish the active dataset, revision, indexing state, and coverage.
3. Open the dashboard with `codex-inspector open`. Reuse the existing loopback server when healthy.
4. Choose the smallest useful scope and filters for the user's question: time range and timezone, project, model, reasoning effort, or contribution kind.
5. Keep the seven Inspector metric families source-controlled: recorded tokens, tokens by contribution kind, tokens over time, token composition, top root sessions, latest capacity observations, and capacity drawdown.
6. Invoke `$codex-inspector:analyze-data-quality` when incomplete indexing, derived values, unsupported sources, or coverage gaps could change the takeaway.
7. Invoke `$codex-inspector:visualize-data` only when a chart materially improves the requested comparison or trend.
8. Summarize the selected scope, applied revision, material coverage caveats, and the dashboard view opened.

## Dashboard rules

- Keep latest-capacity cards independent of historical filters; apply time range only to drawdown history.
- Keep root, descendant, Inspector Review, and other/orphan contributions distinct.
- Do not invent a value or chart for unavailable data.
- Do not expose the loopback authentication fragment, cookies, local source paths, prompts, responses, or raw tool output.
- Treat the internal HTTP API as an implementation detail rather than a public integration surface.
