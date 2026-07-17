# Codex Inspector core wireframe contract

This contract narrows the [Build Week plan](../../plans/build-week-plan.md) to the first reviewable user experience. It covers the dashboard, context inspector, and bounded Effectiveness Reviews. Visual design and direct execution of recommended fixes remain intentionally deferred.

## Target user and job

The target user is a Codex power user who wants to understand whether they are using their available capacity effectively across several repositories, models, and delegated agent workflows.

Their primary job is to understand when and how recorded Codex capacity was spent, identify the user-initiated root sessions and descendant-agent work that explain that usage, inspect a specific session's recorded context, and request evidence-backed guidance for improving future work without relying on an opaque score.

## Primary flow

1. Open the Token & Capacity dashboard and inspect current recorded limit utilization.
2. Change the global time range or filter by project, model, reasoning level, or root/spawned work.
3. Understand token volume, composition, and root-versus-descendant attribution.
4. Identify a token-intensive user-initiated root session and open it in the context inspector.
5. Read the root session's causal token map and select a full agent turn.
6. Follow that turn's chronological events and select an event boundary.
7. Inspect exact evidence, token accounting, and the context before or after that boundary.
8. Return to the dashboard or switch directly to another session.

The review flow connects all three surfaces:

1. Start New review from Reviews, or choose Review effectiveness for a root session on Dashboard or Context Inspector.
2. Confirm a single-session or time-period scope, optional project and focus, model, reasoning, and estimated initial input.
3. Preview the fixed kickoff prompt and explicitly start a visible Codex review task.
4. Leave the in-progress report or follow the linked Codex session.
5. Return to a concise completed report containing evidence-backed strengths and improvement opportunities.
6. Follow a citation into the exact Context Inspector boundary or copy a recommendation prompt for Codex.

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
- A secondary Review effectiveness action on each root-session row that opens the shared Review plan with that root tree preselected.
- Add widget belongs to edit mode for the current view; a template is only a starting point for creating a view.
- Widgets can be rearranged by drag-and-drop or keyboard, and each uses the same configuration icon and realistic configuration modal.
- The Metrics Catalog remains available inside metric-selection flows rather than competing with the saved-view model.

### Context inspector

- A dedicated discovery state when Context Inspector is opened without a session deep link.
- Unified local search over recorded session titles, projects, working directories, IDs, user messages, tool results, and descendant content, with explainable match labels.
- User-initiated root sessions as canonical search results; descendant-only matches remain nested and de-emphasized.
- Persistent selected-session identity, active/index freshness, exact ID, project, working directory, and aggregate root/descendant metadata.
- A causal token map whose primary nodes are full agent turns and descendant sessions, with direct and inclusive downstream token accounting.
- A stable absolute token scale, minimap, fit/zoom controls, and manual branch collapsing.
- A persistent-map workspace: selecting a turn opens its chronological event ledger without discarding the session overview.
- Automatic turn focus mode: selecting a turn collapses the full map into a sticky horizontal topology rail and brings the ledger into the primary viewport.
- An inspector-local breadcrumb of Sessions / root session / full agent turn; Dashboard navigation remains exclusively in the top-level navigation.
- A stacked context-accumulation rail aligned to recorded events.
- Functional icons and explicit labels for user, assistant, recorded reasoning summary, model invocation, tool invocation/result, token usage, spawn/return, patch, web search, compaction, and lifecycle events.
- An always-expanded event evidence surface for every event type: exact payload, surrounding model cycle, relevant model context, token accounting, and boundary comparison are visible together without tabs or disclosure actions.
- Complete recorded user, assistant, reasoning-summary, tool input/output, token, compaction, and lifecycle payloads render inline; exceptionally large blocks may scroll internally but are not hidden.
- Ordered literal context blocks with role, source, contribution, and fidelity; category totals and collapsed summaries do not substitute for the content sequence.
- Explicit before/after context snapshots. Context seen by Codex is reserved for recorded model-call boundaries; intermediate state is labeled reconstructed.
- First-class compaction markers and before/after comparisons.
- Dynamic active-session updates that preserve viewport and selection without a hard page refresh.
- Visible fidelity labels: exact, derived, estimated, and unavailable.
- A session-level Review effectiveness action that opens the same shared Review plan without losing the selected root-session identity.

### Reviews

- A third top-level surface organized around explicitly started, saved Effectiveness Reviews.
- A landing page with New review, any in-progress run, and completed or failed review history.
- One shared single-page Review plan reached from Reviews, Dashboard, or Context Inspector.
- Aggregate scope defaults to the last seven days and may be changed to 24 hours or a custom range, with an optional project filter.
- Single-session scope uses one user-initiated root plus its complete descendant tree.
- One visible four-lens rubric: task framing and steering, execution efficiency, delegation and workflow, and reusable leverage.
- No usefulness ratings, scheduled reviews, multiple profiles, maturity levels, or longitudinal scoring in Build Week.
- A visible but fixed kickoff prompt, optional custom focus, high-capability/high-reasoning default, and collapsed Advanced settings.
- An explicit Start review with Codex action with copyable kickoff prompt as fallback.
- A visible originating Codex session identity and simple In progress, Complete, or Failed status.
- A completed immutable report with no more than five evidence-backed strengths or improvement opportunities.
- Findings include impact, plain-language evidence support, compact citations, recommendation, and an action prompt where warranted.
- Citations deep-link into the existing Context Inspector and preserve a Back to review route.
- Copy prompt is the must-ship recommendation action; opening it as a new Codex task remains stretch scope.
- Best-effort rendering with raw artifact, originating task, and retry recovery when structured display fails.

## Structural acceptance criteria

- A reviewer can move from a token-intensive root session to its context in one action.
- A reviewer can find a session from either the dashboard or inspector.
- A reviewer can start the same prefilled single-session Review plan from either the dashboard or inspector.
- Token widgets disclose combined, root, and descendant-agent values where additive attribution applies.
- Capacity is descriptive and does not imply healthy, unhealthy, wasteful, or optimal behavior.
- The global time range and filters are identifiable as dashboard-wide controls.
- A reviewer can discover the supported metric vocabulary and begin the bounded widget wizard.
- A reviewer can create a view from a template or blank canvas without confusing that action with adding a widget.
- A reviewer can enter edit mode, move a widget, configure it, and return to analysis mode.
- A direct Context Inspector entry opens discovery rather than silently choosing a recent session.
- A search result explains why it matched and retains root-session identity when a descendant matched.
- Direct and downstream tokens remain distinguishable without double-counting tool payloads.
- Selecting a full agent turn reveals its chronological events while the map remains available.
- Selecting a turn automatically enters focus mode without requiring manual page scrolling; expanding the topology rail restores the full map without losing selection.
- Sessions and root-session breadcrumbs return to discovery and map overview respectively, while the top navigation owns Dashboard navigation.
- Event icons remain paired with accessible type labels.
- Every event type exposes its exact recorded payload, surrounding model cycle, and relevant ordered model context without an additional click.
- Tool calls expose exact structured input and recorded output; user and assistant messages expose exact text alongside the full locally reconstructible prompt context.
- Canonical model-call snapshots and intermediate reconstructed state are visibly distinct.
- Context additions, removals, compactions, and unavailable gaps are visible in the accumulation rail.
- The current session, turn, event, and before/after boundary remain clear while inspecting evidence.
- A live fixture can append an indexed event without replacing the page or losing selection.
- Populated, loading, empty, and error states are directly reviewable.
- A reviewer can start a default seven-day aggregate review without completing a wizard or providing outcome feedback.
- Review setup makes the selected scope, model, reasoning level, estimated initial input, and Codex invocation explicit before execution.
- Review status remains available after navigation without displaying a fabricated percentage.
- A completed report shows at most five evidence-backed findings without forcing a strength/improvement quota.
- Finding citations open the corresponding Context Inspector evidence and retain a route back to the report.
- Recommendation prompts are copyable, while Inspector never edits configuration or source files itself.
- An unrenderable report retains access to the raw artifact and originating Codex task.
- The primary flow works with keyboard navigation and at desktop and narrow widths.

## Assumptions

- The dashboard is the default surface opened by `codex-inspector open`.
- Deterministic analysis runs locally and is already indexed when the populated state loads.
- Recorded token usage is exact where present; component-level token contribution may be estimated.
- The first Context Inspector focuses on recorded execution, causal token topology, and locally reconstructible context, not recreating a complete server-side prompt.
- The prototype uses a composite fixture: representative turn events are structure-preserving normalized records derived from a real local rollout, while a clearly identified synthetic descendant branch demonstrates topology absent from the sampled session.
- Absolute paths, identifiers, repository/account details, and incidental sensitive values are not copied verbatim into the fixture.
- Layout changes are in-memory wireframe state; persistence, resizing, and collision rules remain implementation concerns.
- The review fixtures demonstrate the launch, status, report, and failure interactions without starting a real Codex task or reading private local review data.
- Review-created sessions are classified separately and excluded from ordinary review scope by default.

## Open questions

- May an individual widget override the global relative-time range?
- Which coverage gaps appear on each widget versus in a data-health drawer?
- Which supported Codex invocation returns a session ID and which surfaces can deep-link to that session?
- What is the smallest versioned review artifact schema that preserves the agreed finding and evidence contract?
