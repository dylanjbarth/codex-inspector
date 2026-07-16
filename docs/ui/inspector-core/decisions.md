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
- Use one selected context component and evidence pane rather than rendering the full raw prompt by default.
- Keep exact, reconstructed, estimated, and unavailable states visible at the component level.
- Defer the optional Codex review flow and the final Findings/Insights naming decision.

## Deferred alternatives

- A dashboard made primarily of finding cards.
- A single blended efficiency, health, or usefulness score.
- Arbitrary SQL or fully unbounded chart configuration.
- Asking for session usefulness feedback after every session.
- A trace-first session explorer as the first drill-down.
- A conversational review setup as the primary plugin surface.
