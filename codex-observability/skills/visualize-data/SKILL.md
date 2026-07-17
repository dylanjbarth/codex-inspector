---
name: visualize-data
description: Design and QA quantitative visualizations for Codex Observability reports and dashboards. Use when a user asks for charts, trends, distributions, comparisons, clearer metric presentation, or visual QA of session and historical Codex data.
---

# Visualize Data

Create visuals only when they make a relationship easier to understand than a metric card, short table, or compact explorer. Use `/codex`-defined meanings and report JSON provenance; never recompute metrics from rendered HTML.

## Workflow

1. Resolve the plugin root as two directories above this `SKILL.md` and run `doctor` before using session data.
2. Generate or inspect JSON for the relevant session or historical dashboard before designing a visual.
3. Choose the smallest useful form:
   - Line chart for ordered change over time.
   - Bar chart for model, tool, category, status, or workspace comparisons.
   - Stacked bar for composition when parts share a common total.
   - Scatter plot only when comparing two quantitative measures across enough observations.
   - Table or metric card when exact lookup matters more than shape.
4. Keep observed, correlated, inferred, and unavailable values visually distinct.
5. Show the population, time range, units, denominator, and material coverage caveat next to the visual.
6. Reconcile every plotted value with the source JSON and retain its provenance locator.

## Rendering contract

- Preserve the bento layout and keep granular records inside the compact explorer.
- Use vanilla HTML, CSS, JavaScript, and inline SVG only; load no external chart library, font, script, style, image, or analytics tag.
- Construct session-derived labels with DOM APIs and `textContent`, not `innerHTML`.
- Provide text labels, accessible contrast, keyboard access, and a non-color cue for every status.
- Avoid crowded legends, decorative charts, 3D effects, and charts with fewer than two meaningful comparable values.
- Never visualize prompt bodies, assistant messages, private reasoning, credentials, or raw tool output.

If a visualization would overstate low-coverage data, keep the underlying metric unavailable and explain why.
