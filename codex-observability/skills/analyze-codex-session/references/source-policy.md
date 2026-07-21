# Source policy

Use sources in this order:

1. Version-matched `/codex` checkout for schemas, semantics, and lifecycle behavior.
2. Read-only local Codex state and rollouts for what happened in the selected session.
3. Official OpenAI documentation when the source does not resolve a public behavior question.
4. Plugin-derived metrics and recommendations, explicitly labeled as correlated or inferred.

The v0.2 contract targets:

- Codex CLI `0.143.0`
- Source tag `rust-v0.143.0`
- Commit `c4d748f586a84a3ed5b6aceb82e9a1db4abb1cda`

Primary source areas:

- `codex-rs/rollout/src/recorder.rs`
- `codex-rs/protocol/src/protocol.rs`
- `codex-rs/state/migrations/`
- `codex-rs/hooks/src/schema.rs`
- `codex-rs/core-skills/src/invocation_utils.rs`
- `codex-rs/core/src/skills.rs`
- `codex-rs/state/migrations/0021_thread_spawn_edges.sql`
- `codex-rs/core/src/tools/handlers/multi_agents_spec.rs`

Official guidance supplies the model-role and delegation policy that the persisted schema does not define:

- [Recommended models](https://learn.chatgpt.com/docs/models#recommended-models)
- [Subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents)

Do not treat hook transcripts as a stable parser contract. Do not treat skill availability as skill invocation. If neither source nor official documentation establishes a claim, report it unavailable.
