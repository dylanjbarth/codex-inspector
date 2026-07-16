# Core wireframe state matrix

| Surface | State | What the user sees | Available action |
| --- | --- | --- | --- |
| Dashboard | Populated | Saved view header, capacity, token attribution, charts, and token-intensive root sessions | Change view/range/filter, inspect a root session, or enter Edit view |
| Dashboard | Filter dropdown | Project, model, reasoning, and session-kind controls under one header action | Apply or reset inherited view filters |
| Dashboard | Create view | Template and blank-canvas starting points plus a view name | Create a saved view or cancel |
| Dashboard | Edit view | Drag handles, Add widget, Done, and configurable canvas widgets | Rearrange, add, configure, remove, or finish editing |
| Dashboard | Blank view | Empty canvas inside Edit view | Add the first widget |
| Dashboard | Widget configuration | Title, visualization, metric, grouping, time, display, comparison, preview, and removal | Save, remove, or cancel |
| Dashboard | Widget wizard | Bounded six-step metric and visualization configuration with preview | Move back/next, save a sample widget, or cancel |
| Dashboard | Metrics Catalog | Searchable grouped metric definitions with sources, dimensions, coverage, and fidelity | Search, review a metric, or close |
| Dashboard | Loading | Local indexing progress and reserved dashboard structure | Wait or keep the page open |
| Dashboard | Empty | No indexed sessions and an explanation of local scanning | Scan local history |
| Dashboard | Error | Unsupported record format with parser and CLI versions | View diagnostic details |
| Context inspector | Populated with finding | Session finder, turn selector, context components, evidence detail | Switch session/turn/component |
| Context inspector | Populated without finding | Recorded context with no notable deterministic finding | Inspect components or switch session |
| Context inspector | Loading | Selected session identity plus context-loading status | Return to dashboard |
| Context inspector | Empty | No context records for the selected session | Switch session |
| Context inspector | Error | Context adapter failure without implying source data loss | View diagnostics or switch session |

The prototype exposes the global populated, loading, empty, and error states through a clearly labeled wireframe control. Dashboard dialogs and edit mode expose view creation, widget configuration, and reversible in-memory layout editing. Session selection demonstrates the populated-with-finding and populated-without-finding context variants.
