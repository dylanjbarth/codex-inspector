# Codex Observability

Codex Observability is a local-first Codex plugin that generates self-contained bento dashboards across retained Codex history and for individual sessions. Version 0.2 reports source-verified token usage, model, reasoning effort, session speed (Fast, Standard, Mixed, or Unavailable), tool activity, subagents, context lifecycle, verification, coverage, provenance, and deterministic evidence-backed coaching. Matched inline SVG icons identify each main dashboard category without loading external assets. Redacted tool previews are rendered as labeled Input and Output panels with automatic JSON, content-block, JavaScript, or text formatting.

Long session reports use a compact advanced explorer: Tools and Timeline tabs render 25 records at a time inside a bounded viewport, with full-dataset search and filters, chronological/newest ordering, compact pagination, and an accessible detail drawer. Every safe record remains available without making the dashboard page thousands of rows tall.

Version 0.2.7 adds seven focused analytical workflows: **Analyze Data Quality**, **Validate Data**, **Visualize Data**, **Build Dashboard**, **Build Report**, **Create Data Context**, and **Design KPIs**. They reuse the same local, source-verified pipeline rather than introducing a second calculation engine. Each workflow preserves `/codex` source precedence, read-only session evidence, explicit provenance, redaction, offline HTML, and the plugin's no-guessing policy.

## Requirements

- Codex CLI `0.143.0`
- A version-matched OpenAI Codex source checkout at `/codex`
- Python 3.11 or newer

The analyzer uses no third-party Python or browser dependencies and makes no network requests.

## Usage

Ask Codex to analyze the latest session, show its token efficiency, or explain its tool calls and failures.

The standard dashboard and session report are observability-only and do not show rating or improvement cards. Coaching is generated only when the user explicitly asks to rate a session or improve their sessions:

Or run the local CLI:

```bash
python3 scripts/codex_observability.py doctor
python3 scripts/codex_observability.py index --limit 50
python3 scripts/codex_observability.py report --session latest
python3 scripts/codex_observability.py report --session latest --coaching
python3 scripts/codex_observability.py index --coaching
python3 scripts/codex_observability.py serve --port 0
python3 scripts/codex_observability.py clean --cache
```

`index` incrementally analyzes every retained active and archived session, then shows the requested number of recent top-level tasks. Standard output is `index.html`; opt-in historical coaching is written separately to `coaching.html`. Session coaching is likewise written to `<session>-coaching.html`, leaving the observational session report unchanged. Reports default to `~/.codex-observability/reports/`; sanitized history summaries default to `~/.codex-observability/cache/history-0.2.sqlite`.

Use `index --rebuild-cache` to force a full refresh. Use `--cache-dir` or `--policy` to override the derived cache or coaching policy. A user policy at `~/.codex-observability/coaching-policy.json` takes precedence over the bundled policy. Use `--source-root` for an explicitly supplied version-matched source checkout when literal `/codex` cannot be provisioned.

## Coaching policy

When explicitly requested, the green **What you did well** and red **What to improve** cards use deterministic rules with session evidence, confidence, and a versioned policy. They never use a model to grade the user, and they are omitted from ordinary dashboard and session-analysis output.

- Luna is matched to clear, repeatable operational signatures.
- Terra is matched to everyday and read-heavy exploratory signatures.
- Sol is matched to complex, open-ended signatures.
- Subagent coaching distinguishes spawned children from internal guardian and review threads.
- Model and delegation advice is suppressed when coverage is insufficient.

The model mapping follows the official [recommended-model guidance](https://learn.chatgpt.com/docs/models#recommended-models), and delegation rules follow the official [subagent guidance](https://learn.chatgpt.com/docs/agent-configuration/subagents). `/codex` remains authoritative for persisted schemas and behavior.

## Data policy

`/codex` defines the supported schemas and behavior. The analyzer reads the newest `state_*.sqlite` database and matched session rollout from `~/.codex` in read-only mode. It does not read authentication data, diagnostic log bodies, prompt history, browser data, raw config values, or shell snapshots.

Default reports omit prompts and assistant-message bodies. Tool arguments and outputs are redacted, truncated, and collapsed. The history cache stores only identifiers, timestamps, numeric metrics, classifications, statuses, and evidence locators; it never stores prompts, responses, titles, commands, complete paths, or tool output. Generated reports and summaries remain local and should still be treated as sensitive developer data.

## Development

```bash
python3 -m unittest discover -s tests -v
python3 /path/to/skill-creator/scripts/quick_validate.py skills/analyze-codex-session
python3 /path/to/skill-creator/scripts/quick_validate.py skills/validate-data
python3 /path/to/plugin-creator/scripts/validate_plugin.py .
```
