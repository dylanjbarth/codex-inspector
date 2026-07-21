## Context Inspector discovery QA

**Comparison target**

- Source visual truth: `/Users/dylanbarth/Desktop/SCR-20260720-syto.png`
- Final desktop screenshot: `/Users/dylanbarth/Code/personal/codex-inspector/output/playwright/context-discovery-final.png`
- Final loading screenshot: `/Users/dylanbarth/Code/personal/codex-inspector/output/playwright/context-discovery-loading-final.png`
- Final mobile screenshot: `/Users/dylanbarth/Code/personal/codex-inspector/output/playwright/context-discovery-mobile-final.png`
- Viewports: 1481 × 1029 desktop and 390 × 844 mobile
- State: populated discovery, retained-results loading, filtered search, and result navigation

**Full-view comparison evidence**

- The annotated source and final browser-rendered desktop screenshot were inspected together at original resolution.
- The crossed-out discovery introduction is removed and the search/results area fills the available production content canvas.
- The implementation intentionally retains the current product sidebar, header, tokens, and index status rather than copying the older shell visible in the annotated source.
- Result rows replace raw environment wrappers with source-backed user-task titles and add project, start time, duration, completed turns, spawned count, stable session ID, token breakdown, and an explicit Open context action.

**Focused region comparison evidence**

- `context-discovery-loading-final.png` shows the button label changing to `Searching…`, a spinner/status message, and prior results retained at full readable contrast.
- A real `merge conflicts` query updated the URL, returned matching rows, and exposed match categories and source-backed snippets.
- `context-discovery-mobile-final.png` shows the search form stacked at 390 px, result metadata wrapping within the card, and no horizontal page overflow (`scrollWidth` 375 at a 390 px viewport).

**Findings**

- No actionable P0, P1, or P2 mismatch remains for discovery.
- Typography uses the established Inspector sans-serif system with clear title, metadata, and token hierarchy.
- Spacing and layout use the full content width while retaining consistent shell gutters and row rhythm.
- Colors and borders follow the existing light shell and dark evidence-card tokens with readable contrast in idle and loading states.
- The source requires no new raster imagery; the existing product logo and Lucide icon assets remain intact.
- Copy and content now lead with friendly task identity and explain exactly where a search match occurred.
- Responsive behavior contains long task titles and session identifiers without clipping or horizontal scrolling.

**Comparison history**

- Pass 1 [P1]: several rows used injected environment/history boilerplate as their title. Fix: ignore system/developer/environment wrappers, attachment wrappers, and known history-preface text when deriving the first user-authored task title.
- Pass 2 [P2]: retained results faded too aggressively during replacement searches. Fix: keep prior results at full contrast while the loading status and disabled button communicate progress.
- Mobile pass [P2]: long identifiers and unbroken title content widened the document beyond the viewport. Fix: constrain grid children and allow title wrapping at arbitrary break points.
- Final evidence: desktop, loading, and mobile captures show no remaining actionable P0/P1/P2 issue.

**Primary interactions tested**

- Initial populated discovery load.
- Keyword search, URL query synchronization, match highlighting, categories, and snippets.
- Pending search state with existing results preserved.
- Result navigation from the first row into its causal context map.
- Desktop and mobile rendering with real indexed data.

**Console errors checked**

- Final clean browser session: 0 errors and 0 warnings.

**Implementation checklist**

- Context routes skip dashboard-only metrics requests, preventing unrelated metrics failures from blocking discovery and reducing startup work.
- Candidate-first backend search and batched token aggregation are active.
- Friendly source-backed titles are covered by adapter and UI tests.
- Go tests, 37 frontend tests, production build, and diff whitespace checks pass.

**Follow-up polish**

- P3: a small number of sessions whose first substantive user input is pasted terminal output still use that terminal excerpt as the honest source-backed title.

final result: passed

---

# Review detail contrast QA — July 21, 2026

**Comparison target**

- Source visual truth: `/var/folders/f9/3l12fkb576q3ykwc52y58bzh0000gn/T/TemporaryItems/NSIRD_screencaptureui_GybA1r/Screenshot 2026-07-21 at 11.30.12 AM.png`
- Implementation screenshot: blocked pending permission to capture the updated local review route with Playwright.
- Viewport: 1920 × 1200 source screenshot.
- State: completed single-session review with one accepted finding.

**Full-view comparison evidence**

- The source screenshot shows the intended layout and content, with insufficient separation between the page canvas and review cards.
- Metadata chips, finding labels, and long-form finding copy are visibly lower contrast than the summary heading and primary actions.

**Focused region comparison evidence**

- The accepted finding card was inspected at original screenshot resolution.
- The implementation increases the card outline to `#9ca5b3`, metadata text to `#424b58`, body copy to `#35404f`, green section labels to `#356b52`, and amber finding type to `#875800`.
- A post-change browser screenshot is still required to confirm cascade, antialiasing, and actual rendered contrast.

**Findings**

- [P2] Updated review-detail contrast has not yet been visually verified in the browser.
  - Fix applied: stronger panel borders, metadata chips, section labels, finding metadata, body text, citations, and availability badges.
  - Blocker: the available local capture path requires Playwright permission under the selected browser policy.

**Required fidelity surfaces**

- Fonts and typography: unchanged intentionally; browser verification pending.
- Spacing and layout rhythm: unchanged intentionally.
- Colors and visual tokens: targeted contrast changes implemented; browser verification pending.
- Image quality and asset fidelity: no new imagery or assets are involved.
- Copy and content: unchanged.

**Comparison history**

- Source pass [P2]: metadata, section labels, body copy, and panel boundaries were too faint.
- Fix: darkened those foregrounds and strengthened card boundaries without changing layout.
- Post-fix visual evidence: blocked pending browser capture.

**Implementation checklist**

- Capture the same completed-review route at the same viewport.
- Compare the accepted finding card against the supplied screenshot.
- Confirm no P0/P1/P2 contrast issue remains, then update this section to passed.

final result: blocked

---

# Causal turn map QA

**Comparison target**

- Source visual truth: `/Users/dylanbarth/Desktop/SCR-20260720-tkbz.png`
- Implementation screenshot: `/Users/dylanbarth/Code/personal/codex-inspector/output/playwright/context-map-turns-pass-2.png`
- Side-by-side comparison: `/Users/dylanbarth/Code/personal/codex-inspector/output/playwright/context-map-comparison-pass-2.png`
- Focus-state screenshot: `/Users/dylanbarth/Code/personal/codex-inspector/output/playwright/context-map-focus-pass-2.png`
- Narrow-state screenshot: `/Users/dylanbarth/Code/personal/codex-inspector/output/playwright/context-map-narrow-pass-2.png`
- Viewport: 1426 × 873 desktop; 900 × 900 narrow
- State: populated root-session overview with 16 recorded turns; selected Turn 3 focus state

**Full-view comparison evidence**

- The source and implementation were rendered together in `context-map-comparison-pass-2.png`.
- Both use a map-first hierarchy with a compact heading and controls, a categorical legend, a token-sized horizontal root-turn lane, a scrollable canvas, and a minimap.
- The implementation intentionally retains the production Inspector shell and dark evidence-workspace palette instead of copying the wireframe's neutral prototype palette.
- The local corpus used for browser verification has no recognized descendant sessions. The spawned-session branch is therefore covered by the turn-map fixture and interaction test rather than fabricated in the production screenshot.

**Focused region comparison evidence**

- The turn lane was inspected at original resolution in `context-map-turns-pass-2.png`; card widths clearly differentiate the 83,336-token turn from 5,003–24,442-token turns, and each card exposes direct/downstream usage plus factual badges.
- The focused workspace was inspected in `context-map-focus-pass-2.png`; the full map becomes a thin sticky topology rail while the chronological ledger and event evidence occupy the primary viewport.
- The narrow layout was inspected in `context-map-narrow-pass-2.png`; toolbar controls wrap, the legend stacks, and the map remains contained in its scrollable viewport.

**Findings**

- No actionable P0, P1, or P2 mismatch remains for the causal turn-map flow.
- P3: Production data does not currently expose a concise source-backed title for each turn, so cards use neutral `Turn N` and `Agent turn N` labels instead of the descriptive task titles shown in the wireframe. This is honest and does not block map navigation.
- Fonts and typography: the implementation uses the established Inspector type system with comparable hierarchy, readable compact metadata, and no clipped card labels.
- Spacing and layout rhythm: map header, legend, lane, branch container, minimap, and focus rail preserve the wireframe's grouping and density.
- Colors and visual tokens: root and spawned turns are categorically distinct within the existing dark Inspector palette; selected, error, and compaction states retain clear contrast.
- Image quality and asset fidelity: the target contains no raster imagery or custom illustration assets. Existing product logo and icon-library assets remain unchanged.
- Copy and content: the map explanation, direct/downstream labels, factual badges, and focus controls communicate the same interaction model as the source.
- Accessibility: turn nodes and branch controls are semantic buttons with selected/expanded state, visible focus treatment, and text labels; the narrow layout retains reachable controls.

**Comparison history**

- Pass 1 finding [P2]: turn-heavy and turn-light cards were too similar in width, weakening the source's primary token-geometry signal.
- Fix: normalized card width across the visible session while preserving a minimum readable card width and maximum canvas width.
- Pass 2 evidence: `context-map-comparison-pass-2.png` shows the heavy first turn substantially wider than the remaining turns, matching the source hierarchy. No actionable P0/P1/P2 findings remain.

**Primary interactions tested**

- Open the root-session causal turn map.
- Inspect the minimap and root/spawned/direct/compaction legend.
- Select a root turn and enter focus mode.
- Keep the complete root and spawned topology available in the focus rail.
- Expand the full session map without clearing the selected turn.
- Collapse and expand a spawned-agent branch.
- Select a spawned-agent turn and route it to the existing ledger.
- Render the overview at a narrow viewport.

**Console errors checked**

- Browser console: 0 errors and 0 warnings in the populated map state.

**Implementation checklist**

- Turn-level map contract and generated client types are current.
- Root and descendant turn lanes render from source-backed topology.
- Direct and downstream token geometry, badges, minimap, branch collapse, focus rail, and turn selection are implemented.
- Go, repository, server, frontend, typecheck, lint, generated-contract, and production-build verification pass.

**Follow-up polish**

- Add concise source-backed turn titles if the normalized fact model later exposes them without reading raw evidence during map rendering.

final result: passed

---

Latest QA status for the active review-detail contrast pass: browser capture pending.

final result: blocked
