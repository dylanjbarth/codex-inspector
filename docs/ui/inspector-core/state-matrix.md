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
| Context inspector | Discovery | Unified search, project/working-directory and date filters, recent canonical root sessions, and explainable match labels | Search, filter, or open a root-session map |
| Context inspector | Root-session overview | Session metadata, causal token map, minimap, stable scale, direct/downstream totals, compaction markers, and no arbitrary turn selection | Pan, zoom, collapse a branch, select a turn, or change session |
| Context inspector | Turn selected | Persistent map plus the full agent turn's chronological event ledger and stacked context-accumulation rail | Select an event or return to the session overview |
| Context inspector | Event selected | Exact event evidence, token attribution, provenance, and before/after context controls | Change boundary, inspect full/raw content, or choose another event |
| Context inspector | Model-call snapshot | Canonical Context seen by Codex label and categorized context composition | Inspect a component or compare the adjacent boundary |
| Context inspector | Intermediate snapshot | Reconstructed context state label and visible fidelity/coverage gaps | Compare before/after or jump to the next model call |
| Context inspector | Compaction selected | Before/after totals, category deltas, preserved/replaced/removed content, and recorded compacted text | Inspect either boundary or return to the ledger |
| Context inspector | Active session | Active/incomplete turn, Follow live, freshness, and gently appended indexed events | Follow live, pause following, or inspect without losing selection |
| Context inspector | Loading | Requested session identity plus local index-loading status | Return to discovery or dashboard |
| Context inspector | Empty | No readable records or no discovery results for the current query | Change search/filter or return to dashboard |
| Context inspector | Error | Index or adapter failure without implying source-data loss | View diagnostics, change session, or return to discovery |

The prototype exposes the global populated, loading, empty, and error states through a clearly labeled wireframe control. Dashboard dialogs and edit mode expose view creation, widget configuration, and reversible in-memory layout editing. Context discovery, overview, turn, event, compaction, and simulated live-update states are reachable through the primary flow.
