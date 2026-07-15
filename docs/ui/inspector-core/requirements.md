# Codex Inspector core wireframe contract

This contract narrows the [Build Week plan](../../plans/build-week-plan.md) to the first reviewable user experience. It covers the dashboard and the context inspector only. Visual design, the model-powered review flow, and fix prompts are intentionally deferred.

## Target user and job

The target user is a developer who uses Codex across several repositories and suspects that configuration or context problems are degrading sessions.

Their primary job is to move from an aggregate signal to a specific session, understand what entered that session's context, and verify the evidence without uploading data or relying on an opaque score.

## Primary flow

1. Open the dashboard and understand recent activity at a glance.
2. Narrow recent sessions by search, project, or time range.
3. Notice a deterministic finding or suspicious session.
4. Open that session in the context inspector.
5. Select a turn and context component.
6. Inspect its source, contribution, fidelity, and related evidence.
7. Return to the dashboard or switch directly to another session.

## Screens

### Dashboard

- Recent aggregate metrics: sessions, turns, tool calls, recorded tokens, and active time.
- Model and reasoning mix as supporting context, not the dominant visualization.
- Deterministic findings that need attention.
- Searchable and filterable recent sessions.
- Direct entry into a selected session's context.

### Context inspector

- Persistent selected-session identity and metadata.
- A nearby session finder for switching without returning to the dashboard.
- Turn selection for viewing context at a specific point in the session.
- Context composition grouped by messages, instructions, tool definitions, and unavailable runtime context.
- Visible fidelity labels: exact, reconstructed, estimated, and unavailable.
- Evidence detail with provenance and a redacted excerpt.

## Structural acceptance criteria

- A reviewer can move from a dashboard finding to its session context in one action.
- A reviewer can find a session from either the dashboard or inspector.
- Aggregate metrics do not collapse into a single health score.
- Context fidelity is visible before reading detailed evidence.
- The current session and turn remain clear while inspecting components.
- Populated, loading, empty, and error states are directly reviewable.
- The primary flow works with keyboard navigation and at desktop and narrow widths.

## Assumptions

- The dashboard is the default surface opened by `codex-inspector open`.
- Deterministic analysis runs locally and is already indexed when the populated state loads.
- Recorded token usage is exact where present; component-level token contribution may be estimated.
- The first context inspector focuses on recorded composition and provenance, not recreating the complete server-side prompt.

## Open questions

- Should deterministic findings be called **Findings**, **Insights**, or **Needs attention**?
- Does the eventual review experience belong inside the context inspector or in a separate Insights/Reviews area?
- Which metric deserves the strongest emphasis on the dashboard: sessions, active time, recorded tokens, or findings?
- Is selecting a turn essential in the first MVP, or is a session-level context snapshot sufficient?
