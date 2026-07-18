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
| Dashboard | Session review entry | Root-session row with Inspect context and Review effectiveness actions | Open the shared Review plan with that root tree preselected |
| Context inspector | Discovery | Unified search, project/working-directory and date filters, recent canonical root sessions, and explainable match labels | Search, filter, or open a root-session map |
| Context inspector | Root-session overview | Session metadata, causal token map, minimap, stable scale, direct/downstream totals, compaction markers, and no arbitrary turn selection | Pan, zoom, collapse a branch, select a turn, or change session |
| Context inspector | Turn focus | Sticky collapsed topology rail, Sessions / root / turn breadcrumb, chronological ledger, event-type key, and selected-turn accounting | Change turns in the rail, select an event, expand the map, or return to discovery |
| Context inspector | Event evidence | Always-expanded exact event payload, surrounding model cycle, relevant ordered context, token accounting, categorized boundary comparison, and coverage gaps | Scroll the complete evidence, search the payload, or select another event |
| Context inspector | Model-call snapshot | Canonical Context seen by Codex label and categorized context composition | Inspect a component or compare the adjacent boundary |
| Context inspector | Intermediate snapshot | Reconstructed context state label and visible fidelity/coverage gaps | Compare before/after or jump to the next model call |
| Context inspector | Compaction selected | Before/after totals, category deltas, preserved/replaced/removed content, and recorded compacted text | Inspect either ordered boundary or return to event content |
| Context inspector | Active session | Active/incomplete turn, Follow live, freshness, and gently appended indexed events | Follow live, pause following, or inspect without losing selection |
| Context inspector | Loading | Requested session identity plus local index-loading status | Return to discovery or dashboard |
| Context inspector | Empty | No readable records or no discovery results for the current query | Change search/filter or return to dashboard |
| Context inspector | Error | Index or adapter failure without implying source-data loss | View diagnostics, change session, or return to discovery |
| Context inspector | Session review entry | Selected root-session identity with Review effectiveness action | Open the shared Review plan with that root tree preselected |
| Reviews | Empty | Effectiveness Review purpose, visible four-lens rubric, and default seven-day scope | Start a first review |
| Reviews | History | New review action, in-progress run, and completed or failed reports | Open a report or create a review |
| Review plan | Aggregate | Seven-day default, optional project/focus, coverage, model, reasoning, input estimate, and fixed prompt preview | Start review with Codex or copy fallback prompt |
| Review plan | Single session | Prefilled root session and descendants with the same configuration contract | Start review with Codex or return to source surface |
| Review detail | In progress | Scope, elapsed time, simple status, and linked Codex session identity | Open Codex task or navigate away |
| Review detail | Complete | Scope summary and up to five evidence-backed findings | Inspect evidence, copy action prompt, or open Codex task |
| Review detail | Failed | Useful rendering or run failure with preserved artifact and task identity | Open task, view raw artifact, or retry rendering |

The prototype exposes the global populated, loading, empty, and error states through a clearly labeled wireframe control. Dashboard dialogs and edit mode expose view creation, widget configuration, and reversible in-memory layout editing. Context discovery, overview, turn, event, compaction, and simulated live-update states are reachable through the primary flow. Reviews exposes aggregate and single-session plans, in-progress and complete reports, evidence drill-down, copied action feedback, and a recoverable failed state.
