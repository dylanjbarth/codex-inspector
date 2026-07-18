# Codex Effectiveness Reviews brainstorming

This document records the product-design grilling session for the model-powered Reviews experience. It complements the [Home Dashboard metrics brainstorming](./codex-home-dashboard-metrics.md) and [Context Inspector session-map brainstorming](./context-inspector-session-map.md).

- **Decision date:** July 16, 2026
- **Status:** Shared product direction for the next low-fidelity Reviews wireframe
- **Build Week unit:** One bounded, explicitly started Effectiveness Review

## Product question

The Reviews experience should help a Codex power user answer:

> Across the work I selected, what am I doing well, what could I do better, and what concrete technique or Codex-assisted change should I try next?

The product should provide actionable, evidence-backed guidance without pretending there is one canonical definition of a great AI engineer. It should help users right-size models, reasoning, tools, context, and delegation for the work rather than treating low token usage as the goal.

A review is a bounded analysis artifact, not a persistent developer grade or personality profile.

## Build Week product boundary

Build Week ships one consistent **Effectiveness Review**. A review:

- targets one session or a selected time period;
- may optionally be narrowed to one project;
- uses one visible rubric and an optional custom focus;
- starts only after the user reviews its scope and model configuration;
- runs as a visible Codex-style task;
- saves an immutable, locally rendered report;
- returns a small set of evidence-backed strengths and improvement opportunities;
- gives the user copyable Codex kickoff prompts for applicable improvements.

The following are explicitly deferred:

- AI-engineer levels, maturity frameworks, or universal grades;
- usefulness, satisfaction, CSAT, star, or Codex-icon ratings;
- daily session-ranking digests;
- scheduled or recurring reviews;
- automatic comparison across reports or progress scoring;
- multiple review profiles;
- a rigid taxonomy of failure modes;
- silent changes to `AGENTS.md`, skills, hooks, tools, or configuration.

The app preserves review history, but each report stands on its own. Longitudinal coaching may be reconsidered only after repeated real-world reviews demonstrate a stable and useful vocabulary.

## Product vocabulary

Use the following hierarchy consistently:

```text
Reviews
  Review report
    Finding
      Evidence
      Recommendation
      Action prompt
```

- **Reviews** is the third top-level product area beside Dashboard and Context Inspector.
- A **review** is an explicitly started Codex task over a bounded scope.
- A **review report** is the immutable structured artifact produced by that task.
- A **finding** is an evidence-backed strength or improvement opportunity.
- **Insights** is not a separate top-level object in the Build Week vocabulary.

## Connected entry points

All review entry points use one shared Review plan rather than separate workflows.

1. **Reviews:** `New review` opens a time-period review by default and also supports a single-session picker.
2. **Dashboard:** a root-session row can open `Review effectiveness` with that session preselected.
3. **Context Inspector:** the selected root session can open `Review effectiveness` with that session preselected.

The session picker reuses the established Context Inspector search behavior. Reviews does not create a second independent session-discovery model.

The Reviews landing page prioritizes:

1. the primary `New review` action;
2. any in-progress review;
3. completed and failed review history.

Review-created Codex sessions are tagged as Inspector reviews and excluded from normal effectiveness-review scopes by default. This prevents recursive review and keeps review work from distorting ordinary usage analysis.

## Review scope

Build Week supports three constraints:

- **Single session:** one user-initiated root session with all descendant work rolled up.
- **Time period:** all eligible user-initiated root sessions active during the selected period.
- **Optional project filter:** narrows a time-period review to one normalized project.

Seven days is the default aggregate period because it is long enough to reveal recurring patterns while remaining bounded. The user may choose 24 hours or a custom range. A single-session review is usually entered from Dashboard or Context Inspector, but remains discoverable from New review.

Arbitrary manual selection of several unrelated sessions is deferred. Multi-day root sessions remain eligible when they had meaningful activity during the selected period; the scope manifest should distinguish period activity from complete-session context.

## No explicit outcome-feedback step

The Build Week review flow does not ask the user to rate session usefulness or satisfaction.

Codex may inspect recorded signals such as:

- the original goal and later corrections;
- test, build, commit, PR, and artifact evidence;
- unresolved failures or abandoned work;
- repeated retries and rework;
- explicit user reactions or indications of resolution;
- whether the final response supported its completion claim.

These signals can support an interpretation, but they cannot prove off-log value. The review should distinguish directly observed outcome evidence from likely or unknown outcomes. It should not equate a pleasant interaction with a useful result, nor should it invent certainty about what happened after a session ended.

Explicit feedback may be layered in later as a correction mechanism if real usage shows that inference regularly misses important context. It is not required to make the initial review valuable.

## One visible review rubric

The Effectiveness Review uses four visible lenses.

### 1. Task framing and steering

Examine how the user communicates goals, constraints, success criteria, decomposition, corrections, and review instructions.

### 2. Execution efficiency

Examine context growth, token concentration, compactions, retries, tool choice, model choice, reasoning level, unresolved work, and avoidable rework.

### 3. Delegation and workflow

Examine when subagents are used, how work is decomposed, whether independent work is parallelized appropriately, coordination overhead, and unnecessary handoffs.

### 4. Reusable leverage

Examine repeated instructions or workflows that may benefit from `AGENTS.md`, a custom skill, a hook, or automation.

The rubric guides investigation; it is not a set of mutually exclusive finding categories. A finding may describe a cross-cutting pattern supported by model, reasoning, tool, context, and agent-topology evidence. It carries one primary lens for organization without fragmenting one systemic problem into several artificial findings.

## Token and capability principle

The goal is task-to-capability fit, not token minimization.

- High token use is not inherently inefficient.
- Do not recommend cheaper models, lower reasoning, fewer agents, or shorter prompts merely because usage is high.
- Routine, bounded, and easily verified work should use proportionate capability.
- Ambiguous, high-risk, or architecturally deep work should receive stronger models and reasoning.
- A session may appropriately escalate or de-escalate between subtasks.
- A superficially cheaper attempt is not efficient when it produces corrections, rework, or a full restart.
- Flag token inefficiency only when evidence connects usage to avoidable behavior such as redundant retries, irrelevant context, duplicated work, unnecessary tool output, poor delegation, or repeated re-explanation.
- Recognize substantial usage as appropriate when the task and achieved evidence justify it.

Model selection, reasoning level, tool selection, and delegation are different expressions of the broader question: was the right capability applied to the work at the right time?

## Review plan

Starting a review uses one calm, single-page Review plan rather than a wizard.

The plan shows:

- single-session or time-period scope;
- optional project filter;
- optional plain-language `Review focus`;
- included session, turn, date, and project coverage;
- the chosen model and reasoning level;
- estimated initial input size;
- the local output that will be created;
- a collapsed preview of the fixed kickoff prompt;
- a clear statement that starting the review invokes Codex.

The prompt is visible but fixed during Build Week. The user may edit only the optional Review focus. Full prompt editing is deferred because it would weaken the consistent report contract and increase rendering failures.

The default is a high-capability, high-reasoning configuration suitable for qualitative synthesis across sessions. Model and reasoning overrides live under Advanced settings and remain visible before execution.

The primary action is `Start review with Codex`. Copying the kickoff prompt is a fallback for launch failure or an unsupported Codex surface, not the normal workflow.

## Review task and launch model

The review should feel like another Codex task rather than an internal model call.

On start, the local Inspector process should:

1. create a saved review record;
2. write a bounded scope manifest;
3. launch a clearly named Codex review task;
4. capture and store its Codex session identity;
5. navigate to the saved review detail in the `In progress` state.

The review task remains visible and may be opened in Codex when the installed surface supports a reliable handoff or deep link. The session ID remains visible and copyable otherwise. The user can continue chatting in that Codex task after its initial review turn.

The task uses ordinary Codex filesystem and shell capabilities to inspect its own relevant session logs. Build Week does not introduce a new Inspector query-tool family or require an MCP server for review. Inspector provides a scope manifest containing stable session identifiers, source-log locators, time boundaries, project constraints, aggregate facts, and the report destination. A bundled review skill supplies the rubric, investigation guidance, evidence-reference format, and report contract.

The task is instructed to inspect only the selected scope and to treat reviewed repositories as read-only. It may write only the designated derived review artifact.

Whether a locally launched process can attach to or open directly in every Codex surface requires an implementation spike. The product contract remains one-click launch with a copyable-prompt fallback.

## Simple lifecycle

Inspector reports only user-meaningful states:

- **In progress**
- **Complete**
- **Failed**

It does not show a fake percentage or a ceremonial multi-phase progress tracker. The review continues when the user navigates elsewhere, and its status remains available from Reviews.

If a report cannot render, the error should identify whether the artifact is missing, incomplete, invalid, or from an unsupported schema. Recovery actions include:

- `Open Codex task` when supported;
- copy the Codex session ID;
- `View raw artifact`;
- `Retry rendering`.

The review should never disappear because its presentation layer failed.

## Authoritative report artifact

The structured report artifact, not the conversational response, is the interface between the Codex task and Inspector.

- The task receives a review ID, a versioned output schema, and a designated output location.
- It writes one authoritative `review.json` artifact in the Inspector-owned review directory.
- Its visible Codex response may be a human-readable summary rather than raw JSON.
- Inspector watches for the artifact and renders it when structurally readable.
- The initial artifact is an immutable snapshot.
- Continuing the conversation in Codex does not silently rewrite the saved report.
- A revised report requires an explicitly started new review run.

The artifact boundary permits a familiar conversational Codex task and a stable Inspector report at the same time.

## Light validation and graceful rendering

The review prompt and bounded evidence scope are the substantive quality contract. Inspector should not become a second semantic judge of Codex's qualitative analysis.

Inspector performs only light technical checks:

- the artifact is valid JSON for a supported schema version;
- required display fields are present;
- evidence references can be resolved into Context Inspector links.

Inspector should make every reasonable effort to render the returned report. It does not suppress, rewrite, or reject a finding because it disagrees with the conclusion. An unresolved citation remains visible and is labeled `Evidence link unavailable`.

If structured rendering fails completely, the raw artifact and originating Codex task remain accessible.

## Report structure

A completed report is concise and structured rather than a long coaching essay.

It contains:

1. review identity, scope, model, reasoning level, and completion time;
2. a short scope summary;
3. no more than five prioritized findings;
4. linked evidence for every finding;
5. recommendations and action prompts where applicable;
6. a link to the originating Codex review task.

Codex actively looks for both strengths and improvement opportunities, but the report does not enforce a quota. It should not manufacture praise or criticism to fill predetermined slots. Findings are prioritized by likely impact, recurrence within the selected scope, and evidence support.

When an actionable improvement exists, the highest-value opportunity should lead. An honest report may contain one strength and two improvements, or another evidence-supported balance.

## Finding contract

Each finding includes:

- **Kind:** strength or improvement opportunity;
- **Primary lens:** one of the four visible review lenses;
- **Observation:** what pattern Codex found;
- **Impact:** why it matters;
- **Evidence support:** how directly the recorded evidence supports it;
- **Evidence references:** session, turn, event, and short literal excerpt where available;
- **Recommendation:** the smallest useful technique, experiment, or persistent change;
- **Action prompt:** a purpose-built Codex kickoff prompt when action is warranted.

Avoid numerical confidence scores. Use plain-language support labels:

- **Directly observed** — explicit recorded behavior or outcome;
- **Strongly supported** — repeated or corroborated evidence;
- **Worth investigating** — a plausible pattern with limited evidence.

The prompt should omit findings with no meaningful evidence rather than producing unsupported coaching.

## Evidence navigation

Review reports do not recreate the full transcript or event-detail interface.

Each finding shows compact citations containing session identity, turn or event type, and a short literal excerpt. Selecting a citation deep-links into the existing Context Inspector at the exact available boundary. Context Inspector preserves a `Back to review` route with the review and finding position.

Recurring findings may cite evidence across several root sessions. Broken or unavailable locators remain visible with an honest fidelity label.

## Recommendation guardrails

Recommendations should be proportional to the breadth of their evidence.

- A well-supported single-session finding may recommend a technique or experiment.
- An `AGENTS.md` change should normally be supported by a recurring instruction, omission, or correction pattern.
- Creating or revising a skill should normally be supported by a repeated workflow with stable steps.
- An automation proposal should normally be supported by repeated execution across sessions.
- A single-session review may mark a persistent change as worth investigating, but its action prompt should ask Codex to inspect broader evidence before editing.

These are review-prompt guardrails, not a large deterministic policy engine inside Inspector.

## Action boundary

Inspector recommends and packages action; it does not silently act.

Every applicable improvement finding can provide a kickoff prompt containing:

- the proposed technique or change;
- why it was recommended;
- relevant evidence references;
- likely files or configuration involved;
- instructions for Codex to verify assumptions before editing.

`Copy prompt` is the must-ship action. `Open as new Codex task` is stretch scope and should appear only if a reliable supported launch mechanism exists. Strength findings do not need artificial action prompts; they may simply identify a practice worth continuing.

## Review history without progress scoring

Reviews preserves completed and failed runs with scope, project, creation time, status, originating Codex session, and finding count. A user may reopen a report or start another review with a similar scope.

Build Week does not compare two reports, calculate improvement percentages, synthesize a developer trajectory, or assign a level. `Review this scope again` may prefill a new Review plan, but the resulting artifact is independent.

## Review trust boundary

- Session indexing and report storage remain local-first.
- The user explicitly starts every Build Week review.
- Starting a review clearly invokes Codex and displays the chosen model and reasoning level.
- Codex receives a bounded manifest and may inspect source logs only within that scope.
- The review task is visible, attributable, and continuable.
- Inspector never applies a recommended change silently.
- Review tasks are excluded from ordinary effectiveness scopes by default.
- Exact, inferred, unresolved, and unavailable evidence states remain honest.

## Conceptual state model

| Surface | State | Primary content | Primary action |
|---|---|---|---|
| Reviews | Empty | Effectiveness Review explanation and default seven-day scope | New review |
| Reviews | History | In-progress review followed by completed and failed reports | New review or open report |
| Review plan | Aggregate | Seven-day scope, optional project/focus, model, reasoning, estimate, prompt preview | Start review with Codex |
| Review plan | Single session | Selected root session and descendants, optional focus, model, reasoning, estimate | Start review with Codex |
| Review detail | In progress | Scope, elapsed time, Codex session identity | Open Codex task or navigate away |
| Review detail | Complete | Scope summary and up to five findings | Inspect evidence or copy action prompt |
| Review detail | Failed | Failure explanation and preserved task/artifact references | Open task, view raw artifact, or retry rendering |

## Implementation spikes and remaining questions

The product direction is settled, but these implementation questions remain:

1. What supported local invocation reliably creates a Codex review session and returns its session ID?
2. Which Codex surfaces support opening or attaching to that session directly?
3. What is the smallest versioned `review.json` schema that supports the complete finding contract?
4. How should the task write the report atomically so Inspector never reads a partial artifact?
5. How should initial input size be estimated while acknowledging that Codex may inspect additional in-scope logs?
6. What default high-capability model and high reasoning level are available in the installed environment?
7. How should cancelation, process failure, and an unfinished Codex turn map to the three user-facing states?
8. Which stable session, turn, and event locators should the manifest expose for durable evidence links?

These are implementation spikes, not reasons to expand the Build Week product scope.

## Next wireframe exercise

Extend the low-fidelity Inspector prototype with the complete connected review loop:

1. Add Reviews as the third top-level navigation item.
2. Add populated and empty Reviews landing states.
3. Add the single-page aggregate Review plan with the seven-day default.
4. Reuse the plan for a session preselected from Dashboard or Context Inspector.
5. Show model, reasoning, estimated initial input, fixed prompt preview, and Start review with Codex.
6. Show a simple in-progress review with its Codex session identity.
7. Render a completed report with realistic strengths, opportunities, evidence citations, and copyable action prompts.
8. Deep-link one citation into the existing Context Inspector and preserve a return path.
9. Show a failed or unrenderable report with useful recovery actions.
