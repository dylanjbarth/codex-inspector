---
name: analyze-data-quality
description: Assess the compatibility, completeness, consistency, freshness, and privacy of local Codex Inspector data before analysis or publication. Use for coverage audits, unsupported or failed sources, incomplete indexing, unavailable metrics, evidence gaps, and disagreements between dashboard, Context Inspector, and Review results.
---

# Analyze Data Quality

Determine whether Inspector's local, derived evidence is fit for the requested use.

## Workflow

1. Run `codex-inspector doctor --json`. If the CLI is missing, invoke `$codex-inspector:setup`. Stop on an unhealthy compatibility, hook, data-home, or review-store check.
2. Run `codex-inspector status --json`. Record the dataset epoch, applied revision, index state, source counts, diagnostic reason codes, and top-level coverage without exposing complete local paths.
3. Use the current indexed snapshot by default. Run `codex-inspector sync --wait` only when the user explicitly asks for a fresh, finite full-history pass; otherwise identify active or queued indexing as a freshness limitation.
4. Open the relevant Inspector surface. Use an explicit opaque session ID when supplied. For the current task, run `codex-inspector open --current-session` and accept discovery fallback; never infer a session from cwd or timestamps.
5. Assess:
   - compatibility and source support;
   - inventory and completed-turn coverage;
   - metric fidelity (`exact`, `derived`, or `unavailable`) and exclusion reasons;
   - Context evidence availability and fingerprint status;
   - Review scope, coverage gaps, citations, and accepted-report state;
   - privacy risk in any proposed export or screenshot.
6. Return `fit`, `fit with limitations`, or `not fit` for the requested use, followed by the material evidence and limitations.

## Rules

- Treat missing evidence as unavailable, never zero.
- Do not read or mutate Codex source files or Inspector's SQLite database directly.
- Do not expose prompts, responses, raw tool output, credentials, full source paths, or session identifiers unless the user explicitly requests that sensitive evidence.
- Do not present partial aggregates as exact or hide diagnostic groups behind a single percentage.
- Keep source-record availability separate from facts pinned to an earlier dataset revision.
