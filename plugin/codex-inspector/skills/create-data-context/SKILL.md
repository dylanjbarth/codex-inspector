---
name: create-data-context
description: Create or update reusable, source-backed definitions for Codex Inspector metrics and provenance. Use when a user explicitly asks to document metric meanings, formulas, grains, filters, time semantics, coverage requirements, source precedence, caveats, or a reusable context for future Inspector analysis.
---

# Create Data Context

Document analytical meaning without copying session content or changing Inspector data.

## Workflow

1. Run `codex-inspector doctor --json` and record the supported CLI, plugin protocol, index schema, source adapter, and compatibility result.
2. Use Inspector's metric catalog and an accepted Review manifest as the controlling definitions. Do not derive definitions from rendered labels alone.
3. Define the intended consumers, decision, population, grain, and time semantics.
4. For every metric, record:
   - name, question, formula version, unit, population, grain, and calendar timezone behavior;
   - controlling Inspector field or metric family and source precedence;
   - applicable filters, exclusions, and contribution attribution;
   - fidelity (`exact`, `derived`, or `unavailable`), numerator/denominator coverage, and evidence locator shape;
   - reconciliation rules, caveats, privacy class, and unsupported cases.
5. Keep recorded tokens, tokens by kind, tokens over time, token composition, top roots, latest capacity, and capacity drawdown as separate definitions.
6. If persistence is requested, write one sanitized Markdown context in the user-selected workspace and re-read it for unsupported claims or sensitive content.

## Boundaries

- Do not edit Codex source data, Inspector data, hooks, or configuration.
- Do not merge metrics with different populations or grains into one KPI.
- Do not make reusable context a prerequisite for ordinary dashboard use.
- Do not store prompts, responses, titles, commands, complete paths, credentials, raw tool output, or private session identifiers.
- Record unresolved definitions as open gaps rather than guessing.
