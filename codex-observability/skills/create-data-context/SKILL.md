---
name: create-data-context
description: Create or update reusable, source-backed Codex Observability metric context and semantic definitions. Use when a user explicitly asks to document metric meanings, source precedence, grains, filters, caveats, provenance classes, or a reusable analysis context for future reports.
---

# Create Data Context

Create reusable analytical context only when explicitly requested. This workflow documents definitions; it does not copy session content or change Codex data.

## Workflow

1. Resolve the plugin root as two directories above this `SKILL.md` and run `doctor`.
2. Inspect the version-matched `/codex` source and the checked-in adapter contract before defining a field.
3. Define the requested analytical area and intended consumers.
4. Document:
   - Supported CLI, source commit, adapter, and policy versions.
   - Metric name, business question, formula, unit, grain, population, and time semantics.
   - Controlling persisted field and source precedence.
   - Evidence locator format and `observed`, `correlated`, `inferred`, or `unavailable` provenance.
   - Valid filters, exclusions, reconciliation rules, coverage requirements, caveats, and redaction class.
5. Validate every definition against `/codex`; use official OpenAI documentation only for unresolved public behavior.
6. If persistence is requested, write a sanitized Markdown context under `<output-dir>/contexts/`. Treat it as a generated report-support artifact.
7. Re-read the saved file and confirm it contains no prompts, responses, titles, commands, complete paths, credentials, raw tool output, or session-specific private content.

## Boundaries

- Do not edit `/codex`, `~/.codex`, Codex configuration, rollouts, or state databases.
- Do not make reusable context a prerequisite for ordinary session analysis.
- Do not merge definitions with different grains or populations into one KPI.
- Record unresolved definitions as open gaps instead of guessing.
