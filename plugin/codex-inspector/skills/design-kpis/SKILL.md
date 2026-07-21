---
name: design-kpis
description: Define evidence-backed KPIs, driver metrics, guardrails, and scorecards from Codex Inspector metrics and Reviews. Use when a user asks what to measure, how to calculate a Codex usage metric, how to evaluate session effectiveness, or how to design an observability KPI framework.
---

# Design KPIs

Design KPIs that can be reproduced from supported Inspector evidence.

## Workflow

1. Run `codex-inspector doctor --json` and `codex-inspector status --json`; stop if compatibility or the active dataset is unhealthy.
2. State the decision or behavior the KPI should support.
3. Define the population, grain, time window, timezone, inclusion rules, exclusions, and minimum coverage before choosing a formula.
4. Select one primary KPI, a small set of drivers, and explicit guardrails.
5. For each metric specify its question, formula, unit, grain, controlling Inspector metric or Review field, filters, evidence locator, fidelity requirement, caveats, and target status.
6. Draw from these evidence families without collapsing them into an opaque score:
   - usage: recorded tokens, composition, contribution kind, time trends, and top roots;
   - capacity: latest observations and reset-partitioned drawdown;
   - reliability: supported/failed sources, indexing completion, metric coverage, and unavailable evidence;
   - workflow quality: the fixed Effectiveness Review lenses and cited findings;
   - privacy: unintended sensitive-data disclosure as a zero-tolerance guardrail.
7. Invoke `$codex-inspector:validate-data` to confirm required fields and formulas. Mark unsupported KPIs unavailable.
8. Recommend targets only when a baseline, comparison population, or explicit objective exists; otherwise label `baseline needed`.

## KPI rules

- Never treat token volume, cache ratio, reasoning effort, model choice, or descendant count alone as good or bad.
- Do not compare materially different tasks without labeling the population difference.
- Prefer rates with visible numerators and denominators over composite scores.
- Keep deterministic metrics separate from qualitative Review findings.
- Preserve coverage and provenance on every scorecard row.
