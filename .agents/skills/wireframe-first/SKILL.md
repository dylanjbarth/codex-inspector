---
name: wireframe-first
description: Turn product requirements, plans, or rough UI ideas into interactive low-fidelity HTML wireframes and iterate on structure before visual design. Use when Codex is asked to prototype, wireframe, explore, or revise a web UI; establish screen hierarchy, user flows, responsive behavior, content, and application states; or prepare an approved structural prototype for later visual polish.
---

# Wireframe First

Build shared product understanding through a working, deliberately low-fidelity UI. Keep structural decisions separate from visual design.

## Operating rules

- Treat the repository and supplied requirements as the source of truth.
- Do not assume another skill, plugin, MCP server, browser, or framework is available.
- Use available browser or testing capabilities when present, but provide a complete fallback when they are absent.
- Preserve the existing application stack and conventions when a prototype already belongs in the product.
- Prefer plain HTML, CSS, and minimal JavaScript for a disposable standalone prototype.
- Do not install dependencies unless the requested interaction cannot be represented reasonably without them.
- State assumptions and unresolved product questions instead of silently inventing requirements.
- Do not apply branding or visual polish until the user explicitly approves the structure or asks to skip the wireframe phase.

## 1. Establish the prototype contract

Read the requirements, plan, relevant repository guidance, and existing UI before editing. Summarize or create the minimum contract needed to prototype:

- target user and primary job;
- main user flow;
- screen or route inventory;
- content hierarchy and primary actions;
- state matrix, including loading, empty, error, populated, disabled, and success states where relevant;
- responsive expectations;
- known constraints, assumptions, and open questions;
- acceptance criteria for structural sign-off.

If missing information would materially change the flow, ask focused questions before implementation. Otherwise, record reasonable assumptions and proceed.

Keep durable artifacts in existing project locations. If the repository has no convention, use:

```text
docs/ui/<feature>/requirements.md
docs/ui/<feature>/state-matrix.md
docs/ui/<feature>/decisions.md
prototypes/<feature>/
```

Create only the artifacts that add value. Do not duplicate an authoritative product plan.

## 2. Choose the implementation shape

Use this order of preference:

1. Extend an existing prototype if one exists.
2. Use the product's current framework when the prototype is intended to evolve into production.
3. Use standalone semantic HTML, CSS, and JavaScript for disposable exploration.

Keep the prototype easy to run and review. Add direct routes, query parameters, fixtures, or visible development controls when reviewers need to reach otherwise difficult states. Do not hide essential review states behind a long setup flow.

## 3. Enforce low fidelity

Use a restrained wireframe vocabulary:

- grayscale or neutral colors plus one functional focus/selection color;
- system typography;
- a small, consistent spacing scale;
- simple borders and flat surfaces;
- clear labels and realistic domain content;
- conventional controls unless a novel interaction is the subject of the prototype;
- minimal motion only when motion communicates behavior under review.

Do not add decorative gradients, illustrations, marketing imagery, elaborate shadows, custom fonts, ornamental icons, branded color systems, or delight animations. Avoid spending time on pixel-perfect styling.

Visual hierarchy is still required. Use spacing, grouping, headings, alignment, and content order to make the interaction understandable without decoration.

## 4. Make the wireframe functional

Implement the smallest complete experience that supports review:

- connect navigation and primary actions;
- use realistic content lengths and labels;
- represent validation and feedback;
- expose the important states from the state matrix;
- support keyboard navigation and visible focus;
- use semantic elements and meaningful accessible names;
- prevent obvious overflow at narrow and wide viewport sizes.

Prefer reversible prototype state in memory or fixtures. Do not connect production data or perform destructive actions unless explicitly requested.

## 5. Verify with available capabilities

Detect capabilities instead of naming tools the team may not have.

If a shared or automated browser is available:

1. Start or confirm the development server.
2. Open the exact local route.
3. Exercise the primary flow and important states.
4. Check narrow and wide viewports.
5. Inspect visible errors, overflow, keyboard behavior, and console failures when supported.
6. Capture screenshots only when they help comparison or review.

If no browser capability is available:

1. Run the relevant build, type, lint, or test checks that exist.
2. Provide the exact local command and URL for manual review.
3. Give a short checklist covering flow, states, responsive layout, and keyboard navigation.
4. State clearly which visual behavior was not directly verified.

Never claim visual verification from source inspection alone.

## 6. Run the review loop

Stop after the first complete reviewable flow rather than polishing it. Ask the user to comment on the rendered interface or provide feedback tied to a screen, state, and desired outcome.

For each feedback round:

1. Group comments into one coherent, reviewable batch.
2. Separate requirement changes from implementation corrections.
3. Preserve approved structure outside the requested scope.
4. Implement and verify the batch.
5. Update the decision record with accepted decisions, rejected alternatives when useful, and unresolved questions.
6. Return the exact route and states to review next.

Prefer outcome-based feedback such as “keep comparison context visible while selecting a session” over isolated styling instructions.

## 7. Gate visual polish

Before transitioning, summarize the approved:

- information architecture;
- screen and component hierarchy;
- user flow and interactions;
- content model;
- state behavior;
- responsive behavior;
- accessibility constraints;
- remaining product questions.

Ask for explicit structural sign-off if the user has not already given it. After sign-off, offer a separate visual-design pass. Use a visual-design skill only if it is actually available; otherwise continue from the approved contract without one.

During visual design, call out any proposed change to the approved structure instead of folding it silently into styling work.

## Completion criteria

Finish the wireframe phase only when:

- the primary flow is navigable;
- important states are reviewable;
- realistic content demonstrates the hierarchy;
- narrow and wide layouts are usable;
- baseline keyboard and semantic accessibility are present;
- assumptions and open questions are visible;
- verification results and limitations are reported;
- the decision record reflects the latest accepted structure;
- visual polish has not obscured unresolved product decisions.
