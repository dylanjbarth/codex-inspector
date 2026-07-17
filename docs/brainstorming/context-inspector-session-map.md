# Context Inspector session-map brainstorming

This document records the product-design grilling session for the Codex Inspector drill-down experience. It is the working rationale behind the Context Inspector wireframe and complements the aggregate dashboard metrics exploration.

## Product question

The Context Inspector should answer this question first:

> What happened throughout this user-initiated session, where were model tokens used, and what recorded context was available at a specific point in the work?

The first view is not a raw prompt, an automatically generated review, or a list of context components. It is a bird's-eye map of the complete root-session tree. The user drills from that map into one full agent turn, its exact chronological events, and finally the context at an explicit event boundary.

## Interaction hierarchy

```text
Session discovery
  -> root-session causal token map
    -> full agent turn
      -> chronological event ledger
        -> recorded event evidence
        -> before/after context snapshot
```

The desktop wireframe should begin with a persistent-map workspace. The map remains visible while the ledger and evidence panes open beneath it. If that proves too cramped in review, full-page drill-down with breadcrumbs remains a fallback rather than the starting assumption.

## Session discovery and entry paths

The product has three intentional entry paths:

1. Opening Codex Inspector normally opens the Dashboard.
2. Opening Context Inspector from primary navigation opens a dedicated discovery view.
3. Invoking Inspector from within a Codex session deep-links directly to that root session and, when possible, its active turn.

Dashboard rows deep-link to the complete root-session map without automatically choosing a turn. A precise content-search result may focus the matching descendant branch or event. A copied deep link may restore the session, turn, event, and before/after boundary.

Discovery uses one local ranked full-text index over readable recorded content. The canonical result is always a user-initiated root session. A match found only in a descendant session remains nested beneath and de-emphasized relative to its root. Every result explains whether it matched a recorded title, user message, project, working directory, tool result, or spawned agent.

Search and filters include:

- recorded friendly session name or deterministic fallback;
- project and working directory;
- relative or absolute session date;
- session ID;
- keyword search over readable recorded session content.

The session-name fallback order is:

1. recorded Codex session title;
2. first substantive user instruction;
3. project, date, and session ID.

Inspector does not add a separate session-renaming feature. Spawned sessions use their recorded agent name or task, with a neutral numbered fallback, and remain attached to their root.

## Unit of work and causal hierarchy

The canonical work unit is a user-initiated root session plus its complete descendant tree. A **full agent turn** begins with a user instruction and ends when control returns to that user. Messages, tool activity, context lifecycle events, and spawned-agent sessions belong inside that turn.

```text
Root session
  Full agent turn
    recorded messages
    tool activity
    context and lifecycle events
    spawned agent session
      full agent turn
        ...
```

The session map combines a causal tree with token-sized regions. Sibling turns preserve their recorded order. Descendant sessions attach to the turn that spawned them and may branch recursively.

## Token-map semantics

The map uses a stable absolute token scale rather than continuously recomputing each node as a percentage of the current session total. This preserves spatial stability while an active session grows.

- Turn and session geometry represents recorded model-token usage.
- A solid inner block represents tokens used directly by the node.
- An outer containing boundary represents the inclusive downstream total.
- The root container's inclusive total is the complete root-session-tree usage.
- Color identifies event or node category; it does not judge quality.
- Sizing, factual badges, and token labels replace subjective labels such as suspicious, inefficient, or wasteful.

Tool payloads are not added again to the recorded model-token total. Tool detail reports two separate measures:

- **Payload introduced:** tokens in the recorded tool output or other content added by that event.
- **Cumulative context burden:** the estimated contribution of that content across later model calls while it remained visible.

The second value is explicitly estimated unless the source format can prove it. The detail pane preserves available breakdowns such as input, cached input, output, and reasoning tokens.

## Default map density and navigation

The opening map shows every root full-agent turn and every spawned-agent session. Messages and tool calls remain collapsed inside their owning turn. Factual badges disclose counts such as tools, errors, retries, descendants, and compactions.

Large sessions use:

- a minimap;
- pan and zoom;
- Fit session, Fit selection, and Reset zoom actions;
- manually collapsible turns and descendant-agent branches;
- collapsed summaries that retain direct and downstream tokens, turn count, tool count, errors, and compactions.

The interface does not automatically group or omit small turns, and it does not automatically select the largest turn.

## Entry-aware selection

- Dashboard root-session drill-down: show the complete map with no turn selected.
- Discovery root-session result: show the complete map with no turn selected.
- Descendant or content match: focus and select the matching branch or event.
- Invocation from an active Codex session: select the active turn and enable Follow live.
- Exact deep link: restore the encoded selection and context boundary.

These rules respond to explicit navigation intent without having Inspector make a quality judgment.

## Live active sessions

Active sessions stream newly indexed local events into the open workspace without a hard page refresh. Updates must preserve selection, zoom, scroll position, expanded branches, and the open detail pane.

- Completed map nodes remain spatially stable.
- The active node grows in place using the stable absolute scale.
- New events and numeric changes use restrained transitions.
- The current full agent turn is marked incomplete.
- Final token totals are withheld until the corresponding usage record is indexed.
- Last-indexed time and active status remain visible.
- Automatic scrolling occurs only when Follow live is enabled.

Polling is an acceptable implementation mechanism as long as the rendered interface updates dynamically rather than replacing the page.

## Chronological event ledger

Selecting a full agent turn opens its exact chronological event ledger. The ledger is the primary turn detail and may include:

- user and assistant messages;
- tool invocations, arguments, results, and errors;
- agent spawn, progress, completion, and return events;
- context additions and removals;
- compactions and other lifecycle events;
- model invocations and recorded usage events;
- the final response that returns control to the user.

Selecting an event opens its recorded payload, source locator, parser/fidelity metadata, and token attribution. Inspector does not generate an automatic narrative summary. A future explicit Review with Codex action may be considered separately, with scope and cost made clear before invocation.

Large recorded messages and tool payloads may use a bounded inline scroll region for performance, but their content is loaded and visible without a disclosure action. Inspector does not add its own redaction; content already truncated or redacted upstream is labeled as such.

## Context accumulation and snapshots

A stacked context-accumulation rail aligns with the event ledger and shows how recorded or reconstructed context changes through the selected full agent turn. Categories include:

- messages;
- project and global instructions;
- tool definitions;
- tool results;
- skills;
- unknown or unavailable context.

Each boundary may show the content added or removed, reconstructed total after the event, and fidelity. The visualization must allow downward steps and coverage gaps; compaction, truncation, replacement, and unavailable server-side context mean context does not always grow monotonically.

The phrase **Context seen by Codex** is reserved for a model-invocation boundary supported by recorded evidence. Selecting other before/after event boundaries shows **Reconstructed context state**. For a tool result, Inspector should show when it entered accumulated context and identify the next model call that consumed it.

The before/after comparison is explicit and visible on the same evidence surface:

- Context before this event explains what was available when Codex acted.
- Context after this event shows what an event added or removed.
- A compact diff identifies entered, removed, compacted, replaced, and unavailable content.

The UI never uses the ambiguous label “context at event.”

## Compaction as a first-class checkpoint

Compactions receive a marker on the bird's-eye map and an explicit ledger event. Their detail compares:

- context immediately before and after;
- absolute and percentage token reduction;
- category-level changes;
- content preserved verbatim;
- content replaced by a recorded compacted summary;
- content no longer present;
- any context Inspector cannot reconstruct.

When the compacted text is recorded, Inspector displays it exactly. It never generates a substitute summary merely to fill the view.

## Determinism and fidelity

The default Inspector is deterministic and evidence-first. It performs no unexpected Codex calls and makes no quality or efficiency judgment.

Every derived value uses one of these labels where relevant:

- **Exact:** present directly in a readable local source.
- **Derived:** computed deterministically from exact records.
- **Estimated:** reconstructed or tokenized from available content.
- **Unavailable:** the local source cannot establish the value.

Descriptive emphasis is allowed: rectangle size, categorical color, literal search highlights, and badges for recorded errors or compactions. Evaluative labels, automatic reviews, and model-generated summaries are outside the initial Context Inspector flow.

## Wireframe acceptance criteria

The next wireframe is structurally successful when a reviewer can:

1. Enter Context Inspector directly and discover a root session by keyword, project or working directory, and date.
2. Understand why a search result matched and whether the match came from a descendant.
3. Enter from a dashboard session without an arbitrary turn being selected.
4. Read the complete causal root/descendant topology and direct versus downstream token usage.
5. Select a full agent turn and inspect its chronological events.
6. Select a model call and see a canonical context snapshot.
7. Select another event and distinguish reconstructed before/after state from context proven to be seen by Codex.
8. Inspect tool payload introduced separately from cumulative context burden.
9. Inspect a compaction before and after.
10. Navigate a large map with zoom, fit, minimap, and branch collapse controls.
11. Observe a simulated live event update without a page refresh or loss of selection.
12. Use the primary flow with a keyboard and at desktop and narrow widths.

## Deferred questions

- Whether a future explicit Review with Codex action belongs in the event detail, a separate Reviews area, or both.
- Which exact tokenizer and source-version rules are required for comparable estimated component contributions.
- Whether cross-session comparison belongs in Context Inspector or remains a dashboard workflow.

## Feedback round: focused turn inspection

The first session-map wireframe review preserved the discovery interaction and causal map, then refined the transition into event-level inspection.

### Navigation hierarchy

The Dashboard remains a top-level destination in primary navigation, so Context Inspector does not repeat a Back to dashboard breadcrumb. The local breadcrumb represents only the inspector hierarchy:

```text
Sessions / <root session> / <full agent turn>
```

- **Sessions** returns to the instant-search discovery result list with its query and filters preserved.
- The root-session crumb returns to the expanded causal map with the previous turn still highlighted.
- The final turn crumb is current-location text.
- The former Find another session button is removed in favor of this hierarchy.

### Automatic turn focus mode

Selecting a full agent turn immediately enters a focused inspection mode. This is a state transition within the same session route, not a drawer and not a third navigable page.

- The full causal map collapses into a thin sticky horizontal topology rail.
- The chronological ledger and context viewer become the primary viewport without requiring the user to discover content below the fold.
- The rail retains the complete session topology, token sizing, selected turn, and active status.
- Selecting another segment in the rail changes turns without leaving focus mode.
- **Expand session map** restores the full map while preserving the selected turn and viewport context.

This gives the spatial focus of a deeper detail view without sacrificing width to a drawer or forcing repeated page navigation.

### Event vocabulary and icons

The event ledger uses a consistent functional icon and explicit type label. The initial vocabulary includes:

- user message;
- assistant message;
- recorded reasoning summary;
- model invocation;
- tool invocation;
- tool result;
- token usage;
- agent spawn and return;
- patch application;
- web search;
- compaction;
- task and lifecycle state.

Icons aid scanning but never replace the accessible text label. Recorded reasoning summaries are shown exactly when present. Inspector does not decrypt, synthesize, or imply access to hidden chain-of-thought content.

### Always-expanded evidence viewer

Selecting any event opens one continuous evidence surface. Event content and model context are not separated behind tabs or disclosure buttons.

The surface renders, in order:

1. **Exact selected event** — the complete locally recorded user message, assistant message, reasoning summary, structured tool input, tool output, token record, compaction record, or lifecycle payload.
2. **Surrounding model cycle** — every recorded event between the relevant user/tool input and the model output or tool boundary, with the selected event highlighted.
3. **Model context** — the ordered locally reconstructible context for the model invocation that produced the event or will consume it.
4. **Token accounting** — input, cached input, output, reasoning-output, and total usage whenever the rollout records them.
5. **Context boundary comparison** — compact before/after category totals and deltas, with compaction comparison when applicable.

The primary context boundary depends on the event's role without changing the information hierarchy:

- user messages and tool results lead to the next model invocation that consumes them;
- assistant messages, reasoning summaries, and tool invocations lead back to the model invocation that produced them;
- model-invocation events show their own input context;
- token, compaction, and lifecycle events show the nearest provable boundary and label intermediate reconstruction explicitly.

Context is rendered in model order as full readable blocks rather than only category totals or collapsed summaries. Each block identifies its role, source, token contribution, and fidelity. Extremely large blocks may use an inline scroll region, but the recorded content is already present and readable without another action. Unavailable server-side material and encrypted reasoning appear as explicit gaps. Inspector never fabricates missing text.

### Structure-preserving real-session fixture

The next wireframe fixture is derived from a real local Codex rollout log for a failed deployment investigation. Before entering the repository, the fixture normalizes repository names, absolute paths, run and job identifiers, session identifiers, and incidental account details.

The sampled log established the real record vocabulary and shapes used in the wireframe:

- separate `user_message`, `agent_message`, and `message` records;
- recorded reasoning summaries plus encrypted reasoning content;
- `function_call` and `function_call_output` pairs;
- custom patch-call and patch-output pairs;
- cumulative `token_count` records with input, cached input, output, reasoning output, total tokens, and model context-window size;
- context compaction, task lifecycle, thread settings, web search, and abort/complete events;
- large tool results that record truncation, original line count, and original token count.

Representative content keeps the actual interaction shape: a user asks Codex to investigate a failed deployment; Codex states its next step; an `exec_command` call retrieves a workflow job log; the first result indicates a running process; a later poll returns a very large truncated log; and token-count records show how cumulative usage changes.

The event ledger and evidence payloads are grounded in those normalized real records. Synthetic data may still be used to demonstrate a descendant-agent topology not present in that particular sampled root session; any such composite fixture should remain visibly described as structural demonstration data rather than source-exact evidence.
