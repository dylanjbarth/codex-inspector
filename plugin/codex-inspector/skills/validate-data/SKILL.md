---
name: validate-data
description: Validate Codex Inspector calculations, classifications, charts, reports, and conclusions against source-backed metrics and frozen Review evidence. Use to QA a report, reconcile token totals, check chart accuracy, audit coverage or provenance, and determine whether a conclusion is supported.
---

# Validate Data

Audit analytical outputs without changing Codex or Inspector data.

## Workflow

1. Run `codex-inspector doctor --json`. A failed compatibility check is a validation failure, not a caveat to bypass.
2. Run `codex-inspector status --json` and capture the dataset epoch, applied revision, index state, and top-level coverage used by the output.
3. Validate controlling metric rules:
   - completed turns alone contribute recorded usage;
   - cumulative snapshots are normalized into non-negative turn deltas and never added directly;
   - `uncached_input = input - cached_input`;
   - `visible_output = output - reasoning_output`;
   - `residual = total - (uncached_input + cached_input + visible_output + reasoning_output)`;
   - project, model, reasoning, and contribution filters do not alter capacity;
   - historical time range constrains drawdown, not latest-capacity cards;
   - capacity lines never join different limit, window, or reset partitions.
4. Check that every displayed value retains formula version, fidelity, coverage, requested/effective time boundary, and exclusion reasons.
5. For Reviews, verify the frozen scope identity, accepted report state, fixed rubric, maximum five findings, and citations restricted to manifest evidence IDs.
6. Reconcile charts, percentages, subtotals, and prose with the controlling Inspector values exactly.
7. Return compact `pass`, `warning`, or `fail` findings with the affected claim, expected rule, observed value, evidence locator, and corrective action.

## Guardrails

- Do not infer correctness from a plausible-looking chart.
- Do not treat missing evidence as zero or partial coverage as exact.
- Do not query or mutate Inspector's SQLite database directly.
- Do not expose prompts, responses, private reasoning, credentials, raw tool output, or complete local paths.
- Mark inconclusive claims unavailable instead of validating them.
