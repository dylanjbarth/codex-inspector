---
name: validate-data
description: Validate Codex Observability calculations, classifications, visualizations, and conclusions against source-verified session evidence. Use when a user asks to QA a report, verify metrics, reconcile token totals, check chart accuracy, audit provenance, or determine whether a conclusion is supported.
---

# Validate Data

Audit analytical outputs without changing the underlying Codex data. Treat version-matched `/codex` source as the meaning contract and the read-only Codex home as observed evidence.

## Workflow

1. Resolve the plugin root as two directories above this `SKILL.md`.
2. Run `python3 <plugin-root>/scripts/codex_observability.py doctor`. A failed compatibility check is a validation failure, not a caveat to bypass.
3. Prefer metadata-only JSON for the audit:

```bash
python3 <plugin-root>/scripts/codex_observability.py report --session latest --content metadata --format json
```

4. Check the controlling totals and formulas:
   - `threads.tokens_used` controls the session total.
   - Only the final cumulative rollout token snapshot controls the breakdown.
   - Cached input never exceeds input; uncached input equals input minus cached input.
   - Tool calls are deduplicated and correlated by stable call identity.
5. Check analytical integrity:
   - Every displayed value has an evidence locator and provenance class.
   - Inferred values are labeled and never presented as observed.
   - Unknown events, malformed lines, orphaned calls, and reconciliation mismatches lower coverage.
   - Charts, percentages, subtotals, and narrative claims reproduce the JSON values exactly.
6. Check privacy: no prompt body, assistant response, private reasoning, credential, secret, raw tool output, or external resource reference may appear.
7. Return a compact `pass`, `warning`, or `fail` finding list with the affected metric, expected rule, observed value, evidence locator, and corrective action.

## Guardrails

- Do not infer correctness from a plausible-looking chart or total.
- Do not treat missing evidence as zero.
- Do not use token volume or cache ratio alone to judge user quality.
- Do not mutate `/codex`, `~/.codex`, rollouts, or the state database.
- If evidence remains inconclusive, mark the claim unavailable instead of validating it.
