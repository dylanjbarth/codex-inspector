# Codex Inspector — OpenAI Build Week plan

**Status:** Updated after team alignment

**Track:** Developer Tools  
**Build window:** July 14–21, 2026  
**Submission deadline:** July 21, 2026 at 5:00 PM PT

> [!IMPORTANT]
> The product thesis, scope, priorities, architecture, and submission strategy remain open for team discussion. The team has aligned on the immediate execution approach: Dylan, Luis, and Woojun will each build an independent MVP on July 15, then demo their work to one another on July 16 and combine the strongest ideas into a shared product direction.

## How this proposal was developed

Codex created this first draft by reading the team's initial Brainstorming tab and then running an interactive `grill-me` session with Dylan to challenge the target audience, product promise, privacy boundary, evidence standard, technical shape, must-ship scope, and potential team lanes. The session used GPT-5.6 Sol at high reasoning effort.

This process produced a starting point for discussion. It did not make decisions on behalf of the team.

## Product thesis

Codex Inspector is a private, local-first observability suite for people who use Codex heavily across multiple projects. It helps AI engineers understand how Codex is configured, what entered a session's context, what the agent did, where time and tokens went, and which evidence-backed change is most likely to improve future outcomes.

The product is not a generic analytics dashboard or a token-saving tool. It is a progressive-disclosure set of power tools:

> Cross-session patterns → session summary → turn trace → recorded context, instructions, tool calls, and token usage.

The initial audience is a developer who uses Codex daily across several repositories and has accumulated global and project instructions, skills, plugins, hooks, or MCP servers. They can feel their workflow degrading but cannot currently see why.

## Build Week story

The three-minute demo should tell one coherent story:

1. A developer opens Codex Inspector and sees recent Codex activity, computed entirely on their machine.
2. Inspector highlights a suspicious session using deterministic evidence.
3. The developer drills from the session into a turn and then into its context composition.
4. The headline finding shows that unrelated instructions or a skill from one project leaked into another project's context.
5. Inspector quantifies the observed effect without reducing the product to a single score.
6. A secondary finding shows repeated tool thrashing, such as switching unnecessarily between a CLI and an MCP tool.
7. The developer configures an optional Codex-powered review, sees a dry run of its scope and estimated input size, and explicitly starts it.
8. Each recommendation cites evidence and offers a purpose-built prompt that the user may send to Codex to make the change. Inspector never changes configuration silently.

The memorable promise is:

> See what Codex saw, understand what Codex did, and improve what happens next.

## Product principles

1. **Private by construction.** No account, cloud backend, telemetry, or session upload. Raw session data remains local.
2. **Passive facts, opt-in judgment.** Deterministic indexing happens automatically and cheaply. Any model-powered review requires explicit user approval.
3. **Evidence before advice.** Every finding must point to session, turn, instruction, tool-call, or timing evidence. Unsupported coaching is omitted.
4. **Progressive disclosure.** The default view is calm and useful; experts can drill into raw recorded details.
5. **No opaque health score.** Findings describe impact across context quality, tool efficiency, time, token usage, and repeatability without pretending those dimensions collapse into one number.
6. **Codex executes fixes.** Inspector packages a focused prompt and evidence; the user decides whether to ask Codex to act.
7. **Honest precision.** Exact recorded values and estimated or reconstructed values must be labeled differently.

## Must-ship scope

### 1. Installable local plugin and CLI

Ship Codex Inspector as a Codex plugin containing:

- lifecycle hooks for incremental collection;
- one or more skills for inspecting sessions, running reviews, and acting on findings;
- plugin metadata and a local/repository marketplace entry;
- the compiled Inspector CLI and dashboard assets, or a reliable installer for them.

The CLI is the local control plane. An MCP server is intentionally not required for the Build Week version.

Proposed commands:

```text
codex-inspector scan                         # deterministically index local history
codex-inspector open                         # start and open the local dashboard
codex-inspector sessions                     # list/filter indexed sessions
codex-inspector inspect <session-or-turn>    # terminal-friendly detail
codex-inspector review --session <id>        # prepare a targeted Codex review
codex-inspector review --since 7d            # prepare a time-window review
codex-inspector fix <finding-id>              # print or launch the proposed Codex prompt
```

The review command defaults to a dry run. Running the model-powered review requires an explicit confirmation or `--run`-style flag.

### 2. Local ingestion and normalized index

- Discover Codex home from `CODEX_HOME`, falling back to the standard local location.
- Index existing session logs deterministically on first run so the dashboard has immediate value.
- Use hooks to trigger fast incremental indexing after installation rather than repeatedly rescanning everything.
- Normalize session, turn, instruction, tool, usage, model, timing, working-directory, and source metadata into a local SQLite database.
- Keep references to raw records and read full content on demand where possible. Avoid creating unnecessary copies of sensitive transcripts.
- Track parser version and Codex CLI version so format changes fail visibly rather than corrupting results.

The current local session format is not a promised stable hook interface. Treat ingestion as a versioned adapter with fixtures and graceful handling of unknown records.

### 3. Observability experience

The dashboard must support four connected views:

1. **Overview:** recent usage, notable deterministic findings, model/reasoning mix, and links into sessions.
2. **Sessions:** searchable/filterable sessions with project, date, duration, turn count, tool count, and recorded token usage.
3. **Session and turn explorer:** chronological messages, agent activity, tool calls and outputs, compaction markers, timing, and usage.
4. **Context explorer:** recorded instruction layers, messages, available tool definitions, and estimated component contribution to the context window.

Do not claim to display the exact server-side context unless the implementation can prove exact reconstruction. The first engineering spike must classify each component as exact, reconstructed, estimated, or unavailable.

### 4. Two flagship deterministic analyzers

#### Context leakage and instruction hygiene

Detect and explain high-confidence cases such as:

- instructions associated with a different repository appearing in the current project;
- duplicated or conflicting instructions;
- unexpectedly large instruction or tool-definition blocks;
- global guidance that appears overly specific for the current working directory.

The hero demo should use a synthetic, reproducible case with clear provenance. Token contribution by component may be estimated, but the estimate and method must be labeled.

#### Tool thrashing

Detect and explain high-confidence patterns such as:

- repeated failed calls with equivalent inputs;
- alternating between overlapping interfaces for the same service;
- retry loops that end without new information;
- recurring inefficient sequences across multiple sessions.

Start with explicit rules and transparent thresholds. Model-based classification can enrich a finding only after the underlying events are cited.

### 5. Opt-in Codex reviews

Users can target a single session, selected sessions, or a time window and optionally supply a custom review focus.

Before execution, show a dry run containing:

- number of sessions and turns selected;
- date and project coverage;
- deterministic signals that caused data to be included;
- estimated review input size;
- chosen model and reasoning effort;
- review profile or custom focus;
- the local output that will be created.

The review pipeline should produce a minimal evidence bundle, then invoke `codex exec` in an ephemeral, read-only run with a structured output schema. It should not hand every raw transcript to Codex by default. Store the resulting review locally.

Every returned finding must contain:

- observation;
- evidence references, including session and turn identity;
- interpretation and explicit confidence;
- impact dimension;
- recommended change;
- an opt-in fix prompt.

If evidence references cannot be resolved after the review, suppress or visibly reject the finding.

### 6. Judge-ready sample and installation path

- Include a synthetic dataset with seeded context leakage and tool thrashing. It must contain no team session data.
- Provide a one-command sample/demo mode that does not require a judge to expose their own Codex history.
- Provide a supported macOS installation path and a packaged artifact or demo path that does not require judges to rebuild the project from source.
- Explain exactly which Codex versions and local surfaces were tested.

## Explicit non-goals for Build Week

- Cloud sync, accounts, hosted storage, or team dashboards.
- Silent or automatic modification of `AGENTS.md`, skills, hooks, plugins, or MCP configuration.
- A universal quality score for a developer or session.
- Exhaustive coaching for every possible Codex best practice.
- Perfect historical reconstruction across every Codex log version.
- Full semantic session search, bookmarks, or handoff/fork management.
- Compaction steering.
- Windows and Linux support beyond avoiding needless platform coupling.
- A required MCP server.

## Stretch scope

Pursue these only after the complete hero flow works with fresh installation instructions:

1. **Automation recipe:** an optional scheduled weekly review using the bundled review skill.
2. **Sites demo:** a sanitized, sample-data-only hosted demo. Sites must never receive a user's local session data and remains optional because availability depends on plan, region, and workspace settings.
3. **Workflow automation detector:** find repeated tool-call sequences that may deserve a skill.
4. **Instruction change preview:** estimate how a proposed `AGENTS.md` change would affect recent sessions.
5. **Optional MCP adapter:** expose Inspector queries as structured tools if an embedded app or stronger agent interoperability proves valuable after the CLI is stable.

## Recommended implementation shape

Use a TypeScript-first repository to minimize coordination overhead across the CLI, parser, analysis engine, local server, and React dashboard.

```text
Codex session logs / local state
              │
        versioned adapters
              │
       normalized SQLite index
              │
    deterministic analyzers ───────────────┐
              │                            │
      local CLI + HTTP server              │ evidence bundle
              │                            │
       browser dashboard              codex exec review
              │                            │
              └──────── local findings ◀───┘
```

Suggested repository boundaries:

```text
apps/dashboard/             local browser UI
packages/cli/               commands, local server, installation helpers
packages/core/              domain types and orchestration
packages/ingestion/         versioned Codex log/state adapters
packages/analysis/          deterministic analyzers and evidence model
packages/storage/           SQLite schema and migrations
packages/review/            dry-run estimates, evidence bundles, Codex invocation
plugin/codex-inspector/     manifest, hooks, skills, and assets
fixtures/                   synthetic and versioned parser fixtures
docs/                       plans, architecture decisions, demo, and submission notes
```

This structure is a recommendation to confirm during project setup, not a requirement to create empty packages before they are needed.

## Data and trust boundaries

- Bind the dashboard server to loopback only.
- Do not add analytics SDKs or remote fonts/assets that leak usage.
- Never read or display authentication files, environment secrets, or unrelated local files.
- Redact likely secrets from tool inputs/outputs before rendering or building a review bundle.
- Make inclusion rules visible before an AI review runs.
- Use least-privilege, read-only `codex exec` for analysis.
- Keep review runs ephemeral and output-schema constrained.
- Never send session material to Sites or another hosted surface.
- Provide deletion controls for Inspector's derived index and saved reviews without deleting Codex's source logs.

## Evidence model

Use one shared evidence representation for deterministic and model-produced findings. At minimum, an evidence reference should identify:

- session ID;
- turn ID when applicable;
- source record type and stable local locator;
- timestamp;
- redacted excerpt or summary;
- whether the value is exact, reconstructed, or estimated;
- parser/analyzer version.

This is a core product primitive, not implementation polish. It enables trustworthy UI, review validation, reproducible demos, and actionable fix prompts.

## MVP exploration and convergence

Rather than dividing the product into fixed ownership lanes, Dylan, Luis, and Woojun will each independently build a minimum viable version of Codex Inspector on July 15. Each implementation should express its builder's view of the core user problem, hero workflow, product surface, and smallest credible technical path.

On July 16, the team will demo the three MVPs to one another. The goal is not to select a winner wholesale, but to identify the strongest product ideas, interactions, technical approaches, and demo moments across all three. The team will then agree on a shared product vision and continue fleshing out one combined build for the rest of Build Week.

Keep each MVP intentionally small enough to demo in a few minutes. Record important discoveries and tradeoffs so useful ideas survive even when their implementation is not carried forward.

## Seven-day execution plan

### July 14 — align and de-risk

- Discuss the initial product thesis, scope, stack, and naming.
- Spike exact/reconstructed/estimated context visibility against current local records.
- Commit anonymized schema fixtures, never personal transcript content.
- Identify the core questions that independent MVPs should explore.

**Exit gate:** enough shared context for each team member to pursue an informed MVP direction.

### July 15 — build three independent MVPs

- Dylan, Luis, and Woojun each build a minimum viable version of the product.
- Explore the core user problem, hero workflow, product surface, and technical approach independently.
- Keep scope tight enough that each implementation can be demonstrated clearly the next day.
- Capture key discoveries, tradeoffs, and open questions alongside each MVP.

**Exit gate:** three distinct, demoable MVPs that make the team's options concrete.

### July 16 — demo, synthesize, and converge

- Demo all three MVPs to the team.
- Compare the product theses, workflows, interactions, technical choices, and strongest demo moments.
- Select the best ideas from across the implementations rather than adopting one MVP wholesale.
- Align on a shared product vision, combined scope, and immediate integration plan.
- Begin fleshing out the shared build from the chosen ideas.

**Exit gate:** a shared product direction and a concrete plan for the combined build.

### July 17 — complete the analysis loop

- Implement tool-thrashing rules.
- Add review target selection and dry-run estimates.
- Invoke Codex with a minimal evidence bundle and structured output schema.
- Validate returned evidence references before display.

**Exit gate:** targeted review runs end to end and ungrounded findings are rejected.

### July 18 — turn findings into action

- Add finding detail and opt-in fix prompts.
- Package plugin hooks and skills.
- Exercise installation in an isolated `CODEX_HOME`.
- Run the full hero journey from clean sample data.

**Exit gate:** a new tester can install, inspect, review, and produce a fix prompt without team assistance.

### July 19 — dogfood and design polish

- Test on consenting team members' local data without sharing raw sessions.
- Fix top reliability and evidence-quality issues.
- Run a focused design pass for hierarchy, empty states, loading, and readability.
- Capture screenshots and refine the demo dataset.

**Exit gate:** no critical privacy, install, or demo-flow issue remains.

### July 20 — package and tell the story

- Freeze must-ship features.
- Publish the judge-ready artifact or demo path.
- Finish README setup, architecture, privacy, sample-data, supported-platform, and Codex-collaboration sections.
- Record the sub-three-minute demo and prepare the Devpost description.
- Save the required `/feedback` Codex session ID.

**Exit gate:** complete submission rehearsal from a clean machine/profile.

### July 21 — buffer and submit

- Fix only submission-blocking defects.
- Re-run installation and hero-flow checks.
- Verify repository visibility/licensing or judge access.
- Submit well before 5:00 PM PT.

## Acceptance criteria

The Build Week version is complete when:

- a clean macOS user can install and run it from documented steps;
- it performs deterministic historical indexing without invoking a model;
- hooks incrementally capture or trigger indexing for new activity;
- a user can drill from aggregate activity to a session, turn, and recorded context/tool evidence;
- the seeded context-leakage and tool-thrashing findings are reproducible;
- every finding distinguishes fact, estimate, and interpretation;
- an AI review cannot start without an explicit dry run and opt-in;
- the AI review uses a bounded evidence bundle and produces schema-valid local output;
- unresolved evidence references are rejected or clearly marked invalid;
- a finding can generate a focused Codex fix prompt but cannot mutate configuration itself;
- sample mode contains no personal data and works without access to a judge's history;
- the README and demo satisfy the Build Week testing and Codex-collaboration requirements.

## Primary risks and mitigations

| Risk | Mitigation |
| --- | --- |
| The complete context window cannot be reconstructed exactly | Run the Day 1 feasibility spike; label exact/reconstructed/estimated components; demo only supportable claims. |
| Codex local record formats change | Version adapters, retain fixtures by CLI version, tolerate unknown records, and show parser health. |
| AI review becomes expensive | Deterministic prefiltering, minimal evidence bundles, dry-run estimates, explicit scope, and opt-in execution. |
| AI coaching hallucinates | Require resolvable evidence references and confidence; reject unsupported findings. |
| Dashboard becomes a dense metrics wall | Design around the hero journey and progressive disclosure; freeze secondary charts early. |
| Plugin installation does not install CLI runtime cleanly | Prove the clean install on July 18 at the latest; ship compiled assets or a packaged release rather than relying on a source build. |
| Independent MVPs produce incompatible approaches | Compare product and technical choices explicitly during the July 16 demos, preserve useful discoveries, and agree on a shared integration plan before continuing. |
| Personal data leaks into the demo or repository | Use synthetic fixtures only; add secret scanning and a pre-submission privacy review. |

## Decisions still requiring team confirmation

1. Confirm the shared product vision and integration plan after the July 16 MVP demos.
2. Confirm the TypeScript/React/SQLite implementation shape.
3. Choose the final product name and CLI command.
4. Choose the default model and reasoning effort offered by the review dry run.
5. Define the exact supported Codex version range after the ingestion spike.
6. Decide whether the first release is a GitHub artifact, package-registry release, or both.
7. Decide whether an optional Sites sample demo or scheduled review is worth pursuing after the must-ship freeze.

## Sources

- [Team brainstorming document — Brainstorming tab](https://docs.google.com/document/d/1NTR0S1u5Jyj_ihRgM8ITWfauCiNsi5djwG2vAtesUx0/edit?tab=t.1ihbqp5xquqc)
- [OpenAI Build Week overview, requirements, and judging criteria](https://openai.devpost.com/)
- [Build Codex plugins](https://learn.chatgpt.com/docs/build-plugins)
- [Codex hooks](https://learn.chatgpt.com/docs/hooks)
- [Codex non-interactive mode](https://learn.chatgpt.com/docs/non-interactive-mode)
- [Codex scheduled tasks](https://learn.chatgpt.com/docs/automations)
- [Sites](https://learn.chatgpt.com/docs/sites)
