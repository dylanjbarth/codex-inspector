# Codex Inspector core wireframe contract

This contract narrows the [Build Week plan](../../plans/build-week-plan.md) to the first reviewable user experience. It covers the dashboard and the context inspector only. Visual design, the model-powered review flow, and fix prompts are intentionally deferred.

## Target user and job

The target user is a Codex power user who wants to understand whether they are using their available capacity effectively across several repositories, models, and delegated agent workflows.

Their primary job is to understand when and how recorded Codex capacity was spent, identify the user-initiated root sessions and descendant-agent work that explain that usage, and drill into a specific session's recorded context without relying on an opaque score.

## Primary flow

1. Open the Token & Capacity dashboard and inspect current recorded limit utilization.
2. Change the global time range or filter by project, model, reasoning level, or root/spawned work.
3. Understand token volume, composition, and root-versus-descendant attribution.
4. Identify a token-intensive user-initiated root session and open it in the context inspector.
5. Select a turn and context component.
6. Inspect its source, contribution, fidelity, and related evidence.
7. Return to the dashboard or switch directly to another session.

## Screens

### Dashboard

- A global relative-time selector with 24-hour, 7-day, 14-day, 30-day, longer, and custom ranges.
- Global project, model, reasoning-level, and root/spawned filters inherited by widgets.
- Current recorded limit utilization and reset timing, presented descriptively.
- Recorded token total with combined, root-session, and descendant-agent attribution.
- Token volume over time, token composition, and capacity drawdown with reset boundaries.
- The most token-intensive user-initiated root sessions, with all descendants rolled up and separately attributed.
- Direct entry from a contributing root session into its context.
- Visible but shallow customization through templates, a Metrics Catalog, and an Add widget wizard.

### Context inspector

- Persistent selected-session identity and metadata.
- A nearby session finder for switching without returning to the dashboard.
- Turn selection for viewing context at a specific point in the session.
- Context composition grouped by messages, instructions, tool definitions, and unavailable runtime context.
- Visible fidelity labels: exact, reconstructed, estimated, and unavailable.
- Evidence detail with provenance and a redacted excerpt.

## Structural acceptance criteria

- A reviewer can move from a token-intensive root session to its context in one action.
- A reviewer can find a session from either the dashboard or inspector.
- Token widgets disclose combined, root, and descendant-agent values where additive attribution applies.
- Capacity is descriptive and does not imply healthy, unhealthy, wasteful, or optimal behavior.
- The global time range and filters are identifiable as dashboard-wide controls.
- A reviewer can discover the supported metric vocabulary and begin the bounded widget wizard.
- Context fidelity is visible before reading detailed evidence.
- The current session and turn remain clear while inspecting components.
- Populated, loading, empty, and error states are directly reviewable.
- The primary flow works with keyboard navigation and at desktop and narrow widths.

## Assumptions

- The dashboard is the default surface opened by `codex-inspector open`.
- Deterministic analysis runs locally and is already indexed when the populated state loads.
- Recorded token usage is exact where present; component-level token contribution may be estimated.
- The first context inspector focuses on recorded composition and provenance, not recreating the complete server-side prompt.
- The prototype uses synthetic data shaped like locally indexable records; it does not display the inspected user's real session content.
- Full drag-and-drop layout editing and advanced widget configuration remain out of scope for this structural pass.

## Open questions

- Do deterministic Findings/Insights belong on the customizable Home canvas or a dedicated surface?
- Does the eventual review experience belong inside the context inspector or in a separate Insights/Reviews area?
- Is selecting a turn essential in the first MVP, or is a session-level context snapshot sufficient?
- May an individual widget override the global relative-time range?
- Which coverage gaps appear on each widget versus in a data-health drawer?
