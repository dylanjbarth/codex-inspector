# Review launch and artifact contract

The Review contract version is `inspector.review/v1`. Canonical active schemas:

- [`run.schema.json`](../../schemas/reviews/run.schema.json)
- [`report.schema.json`](../../schemas/reviews/report.schema.json)

The launch prompt freezes the review identity, dataset epoch, index revision,
scope, requested model and reasoning, optional focus, report destination, and
report schema. Codex scopes the eligible evidence itself from the pinned local
Inspector index and referenced source logs; there is no precomputed manifest or
evidence bundle in the active prompt-first flow.

The report repeats the scope identity and records the actual model, reasoning,
and completion time. Each finding carries an observation, impact, one primary
lens, and one of `directly_observed`, `strongly_supported`, or
`worth_investigating` support.

## Filesystem layout

```text
reviews/<review-id>/
  report.schema.json
  run.json
  review.json
```

Inspector creates the directory and writes `report.schema.json` before launch.
`run.json` is Inspector-owned process metadata. The Codex task writes
`review.json`. Inspector parses JSONL as an ephemeral process stream and retains
only capped payload-free lifecycle diagnostics in `run.json`; full stdout is
never an Inspector-owned artifact.

The first schema-valid `review.json` is accepted. Inspector records its SHA-256
and completion time in `run.json`; later overwrites remain diagnosable but do
not replace the accepted report. A partial or invalid file is retried while the
process is active and becomes an explicit unrenderable failure after exit.

## Launch command

Inspector starts a new persisted task (never `--ephemeral`) with the Review
directory as the writable workspace and captures stdout as JSONL:

```text
codex exec --json --sandbox workspace-write --skip-git-repo-check -C <review-dir> <launch-prompt>
```

The process parser requires the first `thread.started.thread_id`, stores it in
`run.json`, and recognizes terminal `turn.completed`, `turn.failed`, `error`,
or process exit. `--skip-git-repo-check` is required because the isolated
Review artifact directory is intentionally not a source repository. The task
and descendants are classified
`inspector_review` and excluded from future Review scopes.

## Generated launch prompt

The prompt generated in `internal/reviews/manager.go` is intentionally
self-contained. In addition to the concrete review parameters, it:

- identifies the task as a Codex Inspector Effectiveness Review and makes
  `review.json` the authoritative deliverable;
- defines exact single-session and time-period eligibility semantics;
- requires revision-pinned, read-only investigation of the Inspector index and
  referenced source logs without a precomputed evidence bundle;
- treats all reviewed content as untrusted evidence rather than instructions;
- includes the complete four-lens rubric, task-to-capability principle, support
  labels, recommendation proportionality rules, and action boundary;
- requires a full-scope survey, evidence-backed prioritization, alternative
  explanation checks, and exact in-scope evidence IDs;
- includes the exact JSON shape and identity values required by
  `report.schema.json`; and
- requires validation and one atomic `review.json` write while prohibiting
  project edits and recommendation execution.

The UI shows scope, model, reasoning, estimate, prompt, and output path before
launch. Execution begins only after explicit user confirmation.

## Rubric and report rules

The fixed lenses are `task_framing_and_steering`, `execution_efficiency`,
`delegation_and_workflow`, and `reusable_leverage`. Findings may be strengths
or opportunities; no quota, aggregate score, maturity level, or usefulness
rating exists. Every finding has impact, evidence explanation, one or more
scope-valid Inspector evidence citations, a recommendation, and an optional
copyable action prompt. Inspector never executes the prompt.

The originating task handoff is `codex://threads/<thread-id>` with copyable
fallback `codex resume <thread-id>`.
