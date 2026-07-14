# Wireframe-first UI prototyping with Codex

- **Status:** First draft for team trial
- **Date:** July 14, 2026

## Why we explored this

The team wants a Codex-native way to turn proposed requirements or a product plan into an interactive HTML wireframe, then iterate on the rendered interface until the product structure is understood. Visual design should begin only after the information architecture, flows, states, content hierarchy, and responsive behavior are coherent.

The desired experience is similar to collaborative AI design tools: the human and agent share a reviewable UI artifact, feedback is grounded in the rendered result, and each iteration improves a durable product contract rather than producing disconnected mockups.

## Working conclusion

Use a real local web prototype as the shared artifact. Codex authors and revises the code; reviewers interact with the rendered page and give feedback tied to a screen, state, or region. Keep a small requirements/state/decision trail beside the prototype so accepted decisions survive later implementation and styling work.

This is a workflow, not a dependency on one specific Codex surface. A shared browser with annotations provides the richest loop when available. Browser automation, screenshots, or ordinary manual localhost review are valid fallbacks.

## Decisions from the session

### Wireframe structure before aesthetics

The first prototype should be intentionally low fidelity. It should use neutral colors, system typography, simple borders, realistic content, and minimal motion. It should not use branding, custom fonts, decorative imagery, elaborate shadows, gradients, or polish that can make unresolved product choices feel finished.

Low fidelity does not mean low clarity. Spacing, grouping, alignment, headings, and content order must communicate hierarchy.

### Prototype behavior, not only screenshots

The artifact should be working HTML with navigable flows and reachable states. Important loading, empty, error, populated, disabled, and success states should be directly reviewable instead of appearing only after complicated setup.

Use plain HTML/CSS/JavaScript for disposable exploration. Use the product's existing framework when the prototype is expected to become production code.

### Treat iteration as product discovery

Each feedback round should distinguish:

- a changed requirement;
- a correction to the implementation;
- a visual preference to defer;
- an unresolved product question.

Accepted decisions should be recorded. This prevents later styling work from reopening the entire interaction model unintentionally.

### Make visual polish an explicit phase transition

Before styling, summarize the approved information architecture, component hierarchy, flows, content model, states, responsive rules, and accessibility constraints. Structural changes proposed during polish should be called out rather than hidden inside a visual edit.

### Keep the team workflow self-contained

The team must not depend on skills installed only in one person's Codex environment. The first-draft skill is checked into this repository at `.agents/skills/wireframe-first`, where Codex can discover it for every teammate working in the repo.

The skill does not require named personal skills, browser plugins, MCP servers, or a particular frontend framework. It detects available capabilities and falls back to build checks plus an exact manual review URL and checklist. Optional design or browser skills may enhance the workflow, but they are never prerequisites.

## Proposed operating loop

1. Read the product plan, repository guidance, and related UI.
2. Establish the target user, primary job, flow, screens, state matrix, constraints, assumptions, and structural acceptance criteria.
3. Choose a disposable HTML prototype or the existing application framework based on whether the work should evolve into production.
4. Implement the smallest complete interactive flow under low-fidelity constraints.
5. Verify behavior in an available browser or provide an honest manual-review fallback.
6. Review one coherent feedback batch at a time.
7. Update the prototype and decision record.
8. Repeat until the structure is explicitly approved.
9. Begin a separate visual-design pass using the approved structural contract.

## Suggested artifact layout

Use existing repository conventions when they exist. Otherwise, default to:

```text
docs/ui/<feature>/requirements.md
docs/ui/<feature>/state-matrix.md
docs/ui/<feature>/decisions.md
prototypes/<feature>/
```

Not every exploration needs every file. An authoritative product plan should be referenced rather than copied.

## Example invocation

```text
Use $wireframe-first to turn the supplied product plan into the first
reviewable dashboard flow.

Start by identifying the screens, primary flow, important application states,
assumptions, and open product questions. Then build the smallest interactive
low-fidelity prototype that lets us review the hierarchy and navigation.

Keep it grayscale, use realistic content, and defer branding and visual polish.
Make important states easy to reach. Verify it with the browser capabilities
available in this environment; if none are available, give me the exact local
URL and a manual review checklist. Stop after the first complete flow so the
team can give structural feedback.
```

## Trial questions

The first real prototype should help the team answer:

- Does the skill ask enough product questions without blocking on minor ambiguity?
- Does the low-fidelity constraint keep review focused on structure?
- Are key states easy to reach and compare?
- Does the decision record stay useful without becoming process overhead?
- Does the capability fallback work for teammates without the same installed skills or browser setup?
- What parts of the prototype should survive into production code?

Revise the skill from observed friction after the first one or two real uses rather than trying to encode every possible frontend workflow now.

## Related Codex documentation

- [Preview and annotate local pages with Browser](https://learn.chatgpt.com/docs/browser)
- [Build and share Codex skills](https://learn.chatgpt.com/docs/build-skills)
- [Store durable repository guidance in AGENTS.md](https://learn.chatgpt.com/docs/agent-configuration/agents-md)
