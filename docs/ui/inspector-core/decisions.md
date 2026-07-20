# Core wireframe decisions

## Current decisions

- Use three connected top-level surfaces: Dashboard, Context inspector, and Reviews.
- Use a standalone low-fidelity HTML prototype before choosing the production framework or visual system.
- Make session finding available on both surfaces.
- Make **Token & Capacity** the first dashboard template and recorded tokens the dominant analytical hierarchy.
- Use a user-initiated root session plus its complete descendant tree as the dashboard's unit of work.
- Preserve combined, root-session, and descendant-agent attribution for additive measures.
- Apply one global relative-time range and global project, model, reasoning, and session-kind filters to the dashboard.
- Present recorded capacity utilization, remaining percentage, reset timing, and freshness descriptively without a health judgment.
- Render the capacity hero as one Codex 7-day limit observation. Keep other limit identities and windows out of the hero; they remain available to capacity history.
- Make token-intensive root sessions the primary route from aggregate analysis into the Context inspector.
- Keep token-intensive rows grouped by user-initiated root: show project/task/opaque ID, active interval, agent-tree size, direct-versus-spawned tokens, and both context and review actions in one scan line.
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
- Name the top-level model-powered area Reviews, its saved artifact a review report, and its evidence-backed observations findings.
- Make one bounded review run the Build Week unit; defer developer levels, longitudinal coaching, and universal quality scoring.
- Use one shared Review plan from Reviews, Dashboard, and Context Inspector. Dashboard and inspector entry points preselect the relevant root session.
- Support one root session or a time period with an optional project filter. Default aggregate reviews to the last seven days.
- Use one visible Effectiveness Review rubric with four non-exclusive lenses: task framing and steering, execution efficiency, delegation and workflow, and reusable leverage.
- Remove explicit usefulness ratings, daily ranking digests, recurring schedules, and multiple review profiles from Build Week.
- Treat token efficiency as task-to-capability fit rather than token minimization. Findings may describe cross-cutting model, reasoning, tool, context, and delegation patterns.
- Use a calm single-page plan with optional custom focus, visible scope and estimate, a fixed prompt preview, and collapsed model/reasoning overrides.
- Default the review task to a high-capability, high-reasoning configuration and show that choice before launch.
- Start a visible, continuable Codex-style task with one primary button; copying the kickoff prompt is a fallback.
- Keep review status simple: In progress, Complete, or Failed. Do not show invented percentages or ceremonial phases.
- Make the structured review artifact authoritative and immutable even when the linked Codex conversation continues.
- Render returned reports with light structural validation and graceful fallbacks instead of semantically overruling Codex findings.
- Cap reports at five evidence-backed findings without forcing a strength/improvement quota or rigid failure taxonomy.
- Use Directly observed, Strongly supported, and Worth investigating instead of numerical confidence.
- Show compact citations in Reviews and deep-link full evidence into Context Inspector with a preserved return route.
- Require persistent-change recommendations to be proportionate to recurring evidence through prompt guidance, not a large deterministic policy engine.
- Make Copy prompt the must-ship finding action. Direct creation of a fix task remains stretch scope, and Inspector never applies changes silently.
- Preserve review history without automatic comparison, progress scoring, or scheduled reruns.

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
