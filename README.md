# Codex Inspector

Codex Inspector turns local Codex session metadata into an evidence-backed workflow review for engineers. It first compares recent sessions, then lets the engineer select one workflow and follow its token, turn, tool, compaction, and subagent evidence without leaving the dashboard.

## Product loop

```text
measure -> diagnose -> verify -> improve
```

- **Measure:** token usage, turn counts and duration, compactions, tool outcomes, and agent orchestration.
- **Diagnose:** compare sessions against medians, P90s, ranked workflows, and deterministic signals such as `compaction_churn.v1`.
- **Verify:** select a session and inspect its per-turn token use, timing, tool status, and child-agent footprint.
- **Improve:** ask Codex for an evidence-backed workflow change from the selected session.

Comparisons and Findings are review signals, not proof that a session was inefficient.

## MVP analysis

The interactive `compare → select → explain` flow covers:

- Total token usage across sessions and token use per turn.
- Turn count, total recorded turn time, median duration, and time to first token.
- Compaction count and repeated-compaction review signals.
- Tool-call count, success/completed/failed/incomplete states, and per-turn tool history.
- Longest tool calls when the source log contains measured duration. This is explicitly duration-based; Codex logs do not provide reliable per-tool token attribution.
- Root-versus-subagent token footprint, spawned subagent count, maximum depth, and child-session relationships.

## Run locally

Requirements: Node.js 20 or newer.

```bash
npm install
npm run build
npm test
npm run inspect -- --no-open
```

The fallback command writes `codex-inspector-report.html`. Omit `--no-open` to open it automatically.

## Codex plugin

The repository contains an installable plugin under `plugins/codex-inspector` and a repo marketplace at `.agents/plugins/marketplace.json`.

After installing the local marketplace and plugin, start a new Codex task and prompt:

```text
[@Codex Inspector] Analyze my Codex sessions from the last 30 days
```

The MCP App requests the expanded `fullscreen` display mode used by Data Analytics artifacts. Codex controls the exact placement; unsupported hosts fall back to an inline interactive dashboard.

## Privacy

The report includes only derived counts, cumulative token totals, timing metadata, tool names/categories/outcomes, compaction events, and hashed parent-child session relationships. It excludes prompts, responses, source code, tool arguments, tool results, project paths, usernames, environment variables, and raw session IDs.
