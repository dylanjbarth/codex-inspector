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

The customization flow is separate but uses the same dashboard surface:

1. Create a saved view from an opinionated template or a blank canvas.
2. Enter **Edit view** mode.
3. Add, configure, remove, and rearrange widgets on that view's canvas.
4. Leave edit mode and use the view for analysis.

## Screens

### Dashboard

- A single view header containing the saved-view picker, New view action, global time range, collapsed filters, and Edit view action.
- New views start from Token & Capacity, another supported template, or a blank custom canvas.
- Global project, model, reasoning-level, and root/spawned filters live in a dropdown and are inherited by widgets.
- Current recorded limit utilization and reset timing, presented descriptively.
- Recorded token total with combined, root-session, and descendant-agent attribution.
- Token volume over time, token composition, and capacity drawdown with reset boundaries.
- The most token-intensive user-initiated root sessions, with all descendants rolled up and separately attributed.
- Direct entry from a contributing root session into its context.
- Add widget belongs to edit mode for the current view; a template is only a starting point for creating a view.
- Widgets can be rearranged by drag-and-drop or keyboard, and each uses the same configuration icon and realistic configuration modal.
- The Metrics Catalog remains available inside metric-selection flows rather than competing with the saved-view model.

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
- A reviewer can create a view from a template or blank canvas without confusing that action with adding a widget.
- A reviewer can enter edit mode, move a widget, configure it, and return to analysis mode.
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
- Layout changes are in-memory wireframe state; persistence, resizing, and collision rules remain implementation concerns.

## Open questions

- Do deterministic Findings/Insights belong on the customizable Home canvas or a dedicated surface?
- Does the eventual review experience belong inside the context inspector or in a separate Insights/Reviews area?
- Is selecting a turn essential in the first MVP, or is a session-level context snapshot sufficient?
- May an individual widget override the global relative-time range?
- Which coverage gaps appear on each widget versus in a data-health drawer?
