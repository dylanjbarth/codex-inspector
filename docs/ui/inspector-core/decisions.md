# Core wireframe decisions

## Current decisions

- Start with two connected surfaces: Dashboard and Context inspector.
- Use a standalone low-fidelity HTML prototype before choosing the production framework or visual system.
- Make session finding available on both surfaces.
- Make **Token & Capacity** the first dashboard template and recorded tokens the dominant analytical hierarchy.
- Use a user-initiated root session plus its complete descendant tree as the dashboard's unit of work.
- Preserve combined, root-session, and descendant-agent attribution for additive measures.
- Apply one global relative-time range and global project, model, reasoning, and session-kind filters to the dashboard.
- Present recorded capacity utilization, remaining percentage, reset timing, and freshness descriptively without a health judgment.
- Make token-intensive root sessions the primary route from aggregate analysis into the Context inspector.
- Treat a dashboard as a saved **view**. Templates and blank canvases are starting points for view creation, not peers of Add widget.
- Consolidate the view picker, New view, global time, collapsed filters, and Edit view into one header.
- Put Add widget, drag-and-drop rearrangement, and Done inside an explicit edit mode for the current view.
- Give every widget one standard configuration control that opens a widget-specific modal; the Metrics Catalog is supporting reference material, not the configuration destination.
- Keep customization bounded to tested widget types, documented metrics, deterministic configuration controls, and a guided six-step Add widget flow.
- Make a causal token map the Context Inspector's opening hierarchy. Full agent turns preserve recorded order and descendant sessions attach to the turn that spawned them.
- Use a persistent-map workspace on desktop: map first, chronological event ledger second, event evidence and context snapshots third.
- Use recorded model-token usage for map geometry, with direct usage inside an inclusive downstream boundary. Keep tool payload introduced and cumulative context burden separate to prevent double-counting.
- Open a dedicated session-discovery view when no deep link is supplied. Use one local ranked index, canonical root-session results, de-emphasized descendant matches, and explainable match labels.
- Keep session naming read-only: recorded title, first substantive user instruction, then project/date/ID fallback.
- Use a stable absolute token scale, minimap, fit/zoom controls, and manual branch collapsing so active updates do not reflow completed work.
- Stream newly indexed active-session events into the open workspace without a full page refresh; preserve selection and viewport, and follow automatically only when Follow live is enabled.
- Make the chronological event ledger the primary turn detail. Do not invoke Codex to generate default summaries or judgments.
- Align a stacked categorized context-accumulation rail with ledger events. Reserve Context seen by Codex for model-call boundaries and label intermediate state reconstructed.
- Make compactions first-class map and ledger events with exact recorded before/after comparison when source coverage permits.
- Show exact recorded event content without Inspector-added redaction, using bounded previews and explicit full/raw-content actions for large payloads.
- Keep exact, derived, estimated, and unavailable states visible at the relevant value or boundary.
- Avoid automatic turn selection and evaluative labels. Entry points may restore an explicitly identified branch, turn, event, or active session.
- Remove the redundant Back to dashboard breadcrumb. Use top-level navigation for Dashboard and an inspector-local Sessions / root session / turn breadcrumb for backward movement.
- Enter turn focus mode automatically when a map node is selected. Collapse the full map into a sticky horizontal topology rail rather than using a drawer or a separate third page.
- Replace the tabbed Event content / Context before / Context after interaction with one always-expanded evidence surface for every event type.
- Render the exact event payload, surrounding model cycle, relevant ordered context blocks, token accounting, and compact boundary comparison together. Large blocks may scroll inline, but content is not gated behind another action.
- Pair every event type with a functional icon and an explicit accessible label; distinguish recorded reasoning summaries from unavailable encrypted reasoning content.
- Ground representative event fixtures in normalized real rollout records while keeping source paths, IDs, repository/account details, and incidental sensitive data out of the repository.
- Defer the optional Codex review flow and the final Findings/Insights naming decision.

## Deferred alternatives

- A dashboard made primarily of finding cards.
- A single blended efficiency, health, or usefulness score.
- Arbitrary SQL or fully unbounded chart configuration.
- Asking for session usefulness feedback after every session.
- A trace-first session explorer as the first drill-down.
- A component-inventory-first Context Inspector.
- Constant percentage-based map resizing during active sessions.
- Independent top-level search results for every descendant agent session.
- A conversational review setup as the primary plugin surface.
