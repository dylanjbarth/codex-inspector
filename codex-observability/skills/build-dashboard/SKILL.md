---
name: build-dashboard
description: Generate and QA source-backed Codex Observability HTML dashboards for one session or retained history. Use when a user asks to build, refresh, open, redesign, or inspect a Codex metrics dashboard, scorecard, or monitoring view.
---

# Build Dashboard

Build the requested dashboard through the existing local pipeline. Preserve `/codex` source definitions, read-only session evidence, deterministic rendering, and provenance.

## Workflow

1. Resolve the plugin root as two directories above this `SKILL.md` and run `doctor`.
2. Choose exactly one dashboard scope:
   - Historical: `index --limit 50`.
   - Individual session: `report --session latest|<uuid>`.
3. Default to redacted HTML. Add `--coaching` only when the user explicitly asks for rating or improvement guidance.
4. Generate the dashboard with the bundled CLI and write only under the configured Codex Observability report directory.
5. Check data quality before highlighting a metric. Use `$analyze-data-quality` when reliability warnings or source disagreements are material.
6. Apply `$visualize-data` when a chart materially improves comparison or trend understanding.
7. Verify the output locally: CSP, no network resources, accessible headings and controls, one safely encoded report dataset, and correct provenance links.
8. When asked to open it, serve the report on `127.0.0.1` with `serve --port 0` and open the printed loopback URL.

## Dashboard contract

- Use a responsive bento hierarchy with a small number of primary cards.
- Keep tools and timeline records in the bounded 25-row explorer.
- Put dense tables, provenance, cache diagnostics, and raw coverage details in advanced sections.
- Keep ordinary dashboards observational; coaching cards remain opt-in.
- Never display prompts, responses, private reasoning, secrets, or complete tool output.
- Do not invent a metric to fill an empty tile.
