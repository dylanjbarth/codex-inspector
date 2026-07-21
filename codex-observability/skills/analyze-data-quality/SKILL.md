---
name: analyze-data-quality
description: Assess the quality, completeness, consistency, and privacy of Codex Observability session and history data before analysis or publication. Use for coverage audits, malformed or unknown events, token mismatches, orphaned tools, cache freshness, schema compatibility, and source disagreements.
---

# Analyze Data Quality

Determine whether the available Codex evidence is fit for the requested analysis. `/codex` defines schemas and behavior; `~/.codex` supplies read-only observations.

## Workflow

1. Resolve the plugin root as two directories above this `SKILL.md`.
2. Run `doctor`; record the CLI version, `/codex` commit, adapter version, state schema result, and path accessibility.
3. Generate metadata-only JSON for the selected session or historical population.
4. Assess these dimensions:
   - **Compatibility:** supported CLI, source commit, and required state columns.
   - **Completeness:** rollout availability, final token snapshot, malformed lines, unknown events, missing timestamps, and archived coverage.
   - **Consistency:** state-versus-rollout tokens, duplicated calls, unmatched starts or outputs, parent-child population reconciliation, and aggregate-versus-session totals.
   - **Freshness and identity:** session fingerprint, rollout size and modification time, cache policy version, and deleted-session cleanup.
   - **Privacy:** only sanitized identifiers, timestamps, categories, numeric metrics, statuses, coverage, and evidence locators enter derived history data.
5. Classify each material field's provenance as `observed`, `correlated`, `inferred`, or `unavailable`.
6. Report an overall fitness level—`fit`, `fit with limitations`, or `not fit`—for the specific requested use, followed by evidence-backed limitations.

## Rules

- Do not repair, rewrite, or normalize Codex-owned files.
- Do not convert an incomplete lifecycle into a success or failure without structured evidence.
- Do not hide quality problems behind aggregate percentages.
- Do not publish a comparison when population definitions or denominators differ materially.
- When `/codex` cannot resolve a field, use official OpenAI documentation; if still unresolved, mark it unavailable.
