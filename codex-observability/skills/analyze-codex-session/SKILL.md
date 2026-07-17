---
name: analyze-codex-session
description: Generate and explain source-verified, local Codex session observability reports. Use when a developer or power user asks to analyze or rate a Codex session, improve their Codex sessions, inspect token or cache usage, review models or reasoning effort, audit tool calls and failures, examine compaction or subagents, verify session-data provenance, compare state and rollout totals, or open the Codex Observability HTML dashboard.
---

# Analyze Codex Session

Generate reports with the bundled standard-library CLI. Treat `/codex` as the semantic source of truth and the user's Codex home as read-only session evidence.

## Workflow

1. Resolve the plugin root as two directories above this `SKILL.md`.
2. Run `doctor` before generating a report. Do not bypass a source, runtime, or state-schema mismatch.
3. Use `index` for the all-history bento dashboard. Use `report --session latest` only when latest is the intended session; otherwise pass its UUID.
4. Coaching is opt-in. Add `--coaching` only when the user explicitly asks to rate, grade, score, or improve a session. For improvement advice across session history, use `index --coaching`. Never add coaching for ordinary dashboard, observability, metric, or session-analysis requests.
5. Coaching-enabled output is written separately (`coaching.html` or `<session>-coaching.html`) so it never replaces the standard dashboard or session report.
6. Default to `--content redacted`. Use `metadata` when the user requests the strictest privacy mode.
7. Report the generated absolute HTML path. When the user wants it opened in Codex, run `serve --port 0`, keep the loopback process alive, and open the printed `127.0.0.1` URL with the available browser capability.
8. Explain gaps from Reliability and Provenance. Never invent unavailable data.

## Analytical skill routing

Use the smallest supporting skill set needed for the request:

- `$analyze-data-quality` for source coverage, consistency, freshness, compatibility, and privacy fitness.
- `$validate-data` for QA of formulas, charts, claims, and provenance.
- `$visualize-data` for quantitative chart selection and visual QA.
- `$build-dashboard` for a historical or per-session dashboard.
- `$build-report` for one durable analytical report artifact.
- `$create-data-context` only for an explicit reusable metric or semantic-context request.
- `$design-kpis` for KPI definitions, drivers, guardrails, targets, and scorecards.

Run quality analysis before validation when source fitness is uncertain. Run validation after report or dashboard generation when the user asks for an audit. These skills do not broaden access to additional files or permit network requests.

Invoke the CLI with:

```bash
python3 <plugin-root>/scripts/codex_observability.py doctor
python3 <plugin-root>/scripts/codex_observability.py index --limit 50
python3 <plugin-root>/scripts/codex_observability.py report --session latest
python3 <plugin-root>/scripts/codex_observability.py report --session latest --coaching
python3 <plugin-root>/scripts/codex_observability.py index --coaching
python3 <plugin-root>/scripts/codex_observability.py serve --port 0
python3 <plugin-root>/scripts/codex_observability.py clean --older-than 30
python3 <plugin-root>/scripts/codex_observability.py clean --cache
```

Common overrides:

- `--source-root /codex`
- `--codex-home "${CODEX_HOME:-$HOME/.codex}"`
- `--output-dir "$HOME/.codex-observability/reports"`
- `--cache-dir "$HOME/.codex-observability/cache"`
- `--policy /path/to/coaching-policy.json`
- `--format html|json`
- `--content metadata|redacted`
- `--coaching` only for an explicit rating or improvement request

## Source and documentation policy

Read [references/source-policy.md](references/source-policy.md) when `doctor` fails, an event is unknown, or the user questions a metric's meaning. Inspect the version-matched `/codex` files named in the contract before consulting documentation.

If `/codex` does not resolve the question, use the `openai-docs` skill and only official OpenAI documentation. Documentation can clarify a public contract; it cannot override contradictory version-matched source or manufacture missing session evidence.

Read [references/metrics.md](references/metrics.md) when explaining token formulas, tool correlation, coverage, or coaching.

## Privacy

- Do not read `auth.json`, diagnostic log bodies, prompt history, browser data, config values, or shell snapshots.
- Do not print prompts, assistant messages, private reasoning, raw environment variables, or full tool output.
- Keep the state database and rollouts read-only.
- Do not make network requests from the analyzer.
- Treat generated reports as sensitive local developer data.
- The derived cache may contain only sanitized summaries; never add prompts, responses, titles, commands, complete paths, or raw tool output.
- Explain that “under the hood” means observable operational events, not private chain-of-thought.

## Failure handling

- Missing `/codex`: stop and report the exact path; accept `--source-root` only when the user supplies a version-matched alternative.
- Version or commit mismatch: fail closed and explain the expected and observed values.
- Unknown event: preserve the type, lower coverage, and continue with supported metrics.
- Missing token breakdown: report the authoritative state total and mark the breakdown partial.
- State/rollout mismatch: keep the state total, show both values, and flag reconciliation.
- Inconclusive task or model fit: suppress coaching instead of guessing.
