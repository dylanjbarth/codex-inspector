---
name: design-kpis
description: Define evidence-backed KPIs, driver metrics, guardrails, and scorecards for Codex usage and Codex Observability quality. Use when a user asks what to measure, how to calculate a metric, how to evaluate session effectiveness, or how to design an observability KPI framework.
---

# Design KPIs

Design KPIs that can be reproduced from supported Codex evidence. Use `/codex` for metric meaning and preserve provenance from source field to scorecard.

## Workflow

1. State the decision or behavior the KPI should support.
2. Define the population, grain, time window, inclusion rules, and exclusions before choosing a formula.
3. Select one primary KPI, a small set of drivers, and guardrails.
4. For each metric specify: name, question answered, formula, unit, grain, controlling source, evidence locator, coverage requirement, caveats, and target status.
5. Separate these metric families:
   - **Accuracy:** state-rollout reconciliation, provenance coverage, and formula validation.
   - **Reliability:** malformed-event rate, lifecycle correlation, unknown-event coverage, and report completion.
   - **Workflow quality:** verification after changes, recovered retries, and completed delegated work.
   - **Efficiency context:** tokens per turn or tool, cache ratio, retries, and compactions.
   - **Privacy:** redaction failures and external-resource references, both with a zero-tolerance guardrail.
6. Validate that the required fields exist for the supported adapter. Mark unsupported KPIs unavailable.
7. Recommend targets only when a baseline, comparison population, or explicit product objective exists; otherwise label them `baseline needed`.

## KPI rules

- Never treat token volume, cache ratio, model choice, or subagent count alone as good or bad.
- Do not compare sessions with materially different task signatures without labeling the difference.
- Prefer rates with visible numerators and denominators over opaque scores.
- Keep observed metrics separate from deterministic coaching judgments.
- Every KPI card or scorecard row must link back to provenance and coverage.
