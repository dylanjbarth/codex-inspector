---
name: build-report
description: Turn reviewed Codex session evidence into one durable, source-backed HTML or JSON report. Use when a user asks for a session analysis report, historical report, audit export, analytical narrative, or a polished report that explains Codex usage.
---

# Build Report

Produce exactly one requested report surface from the existing Codex Observability pipeline. `/codex` controls metric meaning and every analytical claim must retain provenance.

## Workflow

1. Resolve the plugin root as two directories above this `SKILL.md` and run `doctor`.
2. Confirm the report scope: one session, coaching-enabled session, all history, or coaching-enabled history.
3. Use HTML for a reader-facing report and JSON only for testing, audits, or an explicit machine-readable request.
4. Generate through the bundled CLI rather than manually reproducing calculations:

```bash
python3 <plugin-root>/scripts/codex_observability.py report --session latest --content redacted --format html
python3 <plugin-root>/scripts/codex_observability.py index --limit 50
```

5. Add coaching only for an explicit request to rate or improve Codex usage.
6. Validate material calculations and conclusions with `$validate-data`; run `$analyze-data-quality` when coverage is limited.
7. Use `$visualize-data` only when a visual improves the report's main analytical takeaway.
8. Verify the final artifact opens offline, contains no external resources, and includes the supported adapter, coverage, reliability, privacy statement, and provenance.
9. Return the absolute report path and a concise summary of material limitations.

## Guardrails

- Do not finish with chat prose when the user requested a report artifact.
- Do not create multiple competing report files unless the user explicitly requests variants.
- Do not expose prompt bodies, responses, private reasoning, credentials, or raw tool output.
- Do not present inferred recommendations as observed facts.
- Mark unresolved data unavailable instead of filling narrative gaps.
