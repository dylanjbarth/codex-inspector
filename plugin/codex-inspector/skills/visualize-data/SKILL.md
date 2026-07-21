---
name: visualize-data
description: Design and QA quantitative visualizations from Codex Inspector metrics and Review evidence. Use when a user asks for charts, trends, distributions, comparisons, clearer metric presentation, or visual QA of session and historical Codex data.
---

# Visualize Data

Create a visual only when it makes a relationship easier to understand than a metric card, short table, or Context Inspector view.

## Workflow

1. Run `codex-inspector doctor --json` and `codex-inspector status --json`; stop on unhealthy compatibility or identify incomplete indexing as a limitation.
2. Use values from the Inspector dashboard or a frozen accepted Review manifest. Never recompute metrics from raw Codex rollouts or rendered HTML.
3. Choose the smallest useful form:
   - line chart for ordered change over time;
   - bar chart for project, model, contribution kind, or root-session comparisons;
   - stacked bar for mutually exclusive token composition;
   - small multiples for capacity series partitioned by limit, window, and reset boundary;
   - table or metric card when exact lookup matters more than shape.
4. Keep exact, derived, and unavailable values visually distinct with a non-color cue.
5. Show population, time range, timezone, units, denominator, dataset revision, and material coverage caveats next to the visual.
6. Reconcile every plotted value with its controlling Inspector value and evidence locator. Invoke `$codex-inspector:validate-data` before publishing a custom visual.

## Rendering rules

- Prefer the existing Inspector dashboard when it already answers the question.
- Do not join separate capacity partitions or imply continuity across reset boundaries.
- Provide text labels, accessible contrast, keyboard access where interactive, and a table alternative for exact values.
- Load no external analytics, fonts, scripts, styles, images, or chart data into a local report without explicit approval.
- Never visualize prompts, responses, private reasoning, credentials, full paths, or raw tool output.
- Keep low-coverage values unavailable when visualization would overstate certainty.
