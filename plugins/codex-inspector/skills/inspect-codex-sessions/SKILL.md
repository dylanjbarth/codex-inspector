---
name: inspect-codex-sessions
description: Compare local Codex workflows and inspect token, turn-time, compaction, tool-outcome, or subagent evidence with the Codex Inspector interactive dashboard. Use when an engineer asks to inspect, review, or improve how they use Codex.
---

# Inspect Codex Sessions

Use the bundled `inspect_codex_sessions` MCP tool for Codex usage analysis.

1. Infer the requested recent window in whole days. Use 30 days when the user does not specify one. Accept only 1–365 days.
2. Call `inspect_codex_sessions` once with `since_days`.
3. Let the MCP app render the compare-select-explain dashboard. It requests the Codex expanded display and falls back to inline when the host declines.
4. Summarize only the deterministic metadata result returned by the tool. Do not read or quote raw session files, prompts, responses, source code, tool arguments, tool results, or project paths.
5. Distinguish cross-session comparisons from within-session evidence: rankings and medians compare workflows; turn timelines, tool outcomes, and agent trees explain one selected workflow.
6. Describe `compaction_churn.v1` as a pattern worth reviewing. Never claim that compaction or a high metric proves inefficiency, wasted tokens, or poor productivity.
7. Describe “expensive tool calls” as longest measured calls. Do not imply per-tool token cost because the source data does not support it.
8. If no Finding triggers, report that honestly. Do not weaken the rule or silently extend the selected window.
9. If the tool reports incompatible or unreadable sessions, state the visible coverage limitation before drawing a conclusion.

Useful prompts include:

- “Analyze my Codex sessions from the last 30 days.”
- “Which sessions used the most tokens, and what happened inside them?”
- “Show me incomplete tools and the longest measured tool calls.”
- “How much work did my subagents do?”
