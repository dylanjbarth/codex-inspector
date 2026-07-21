# Compatibility spike — 2026-07-19

This report is payload-safe: it contains aggregate structural cohorts only. It
does not retain source paths, session identifiers, message content, tool
payloads, or evidence bytes.

## Method

The selected effective Codex home was scanned into an isolated temporary
Inspector dataset. Each source was grouped by detected CLI version, source
kind, current state, and the validator's structural rejection reason. The
adapter accepts a source only after its identity, completed-turn lifecycle,
usage, evidence offsets, and lineage-relevant structure pass the existing
full-record validator.

## Observed inventory

| Version | Kind | State / reason | Sources |
| --- | --- | --- | ---: |
| 0.142.5 | active rollout | current | 68 |
| 0.144.1 | active rollout | current | 65 |
| 0.142.4 | active rollout | current | 12 |
| 0.144.0 | active rollout | current | 3 |
| 0.143.0 | active rollout | current | 1 |
| 0.144.1 | active rollout | unsupported: incompatible turn context | 29 |
| 0.142.5 | active rollout | unsupported: incompatible response record | 14 |
| 0.144.1 | active rollout | unsupported: incompatible turn identity | 7 |
| 0.142.4 | active rollout | unsupported: incompatible turn context | 5 |
| 0.142.4 | active rollout | unsupported: incompatible response record | 2 |
| 0.142.5 | active rollout | unsupported: incompatible turn context | 2 |
| 0.144.1 | active rollout | unsupported: incompatible event record | 1 |
| 0.144.1 | active rollout | unsupported: incompatible record envelope | 1 |
| 0.144.1 | active rollout | unsupported: missing required session identity | 1 |

The 211 observed sources are fully accounted for: 149 structurally compatible
and 62 explicitly rejected. The dominant safely compatible cohort is accepted
by the structural fingerprint, not by a version range. The rejected cohorts
remain visible as diagnostics and are not normalized optimistically.

## Fixture and test evidence

`internal/sources/adapter_test.go` creates a normalized synthetic source with
CLI version `0.142.5` and proves that it retains completed turns and exact
evidence. It also proves that the unknown-record fixture remains rejected.
The fact golden was regenerated through
`scripts/phase0/generate-fact-golden.sh`.
