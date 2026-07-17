# Codex Inspector plugin

Codex Inspector is a local-first Codex plugin that compares session metadata and opens an interactive MCP App. The app requests Codex's expanded `fullscreen` display mode, which the desktop host can present as an adjacent artifact panel. If the host declines, the same app remains usable inline.

## Runtime flow

```text
Plugin skill
    -> inspect_codex_sessions (local MCP tool)
    -> ~/.codex/sessions JSONL
    -> metadata-only comparison + session evidence model
    -> MCP UI resource
    -> expanded Codex app panel, with inline fallback
```

The dashboard follows one connected flow: compare sessions, select a workflow, then explain it with per-turn token use, duration, compactions, tool outcomes, and agent orchestration. “Longest tool calls” uses recorded duration because the source logs do not provide trustworthy per-tool token cost.

The deterministic report excludes prompts, responses, source code, shell output, tool arguments, tool results, project paths, usernames, environment variables, and raw session IDs.

## Development

From the repository root:

```bash
npm install
npm run build
npm test
npm run inspect -- --no-open
```

The browser report and MCP App use the same compiled UI bundle. `npm run inspect` is the fallback and test entry point; the installed plugin is the primary product surface.

## MCP surface

- Server: `codexInspector`
- Tool: `inspect_codex_sessions`
- UI resource: `ui://widget/codex-inspector-artifact-0.2.0.html`
- Supported display modes: `inline`, `fullscreen`
- Initial requested mode: `fullscreen`
