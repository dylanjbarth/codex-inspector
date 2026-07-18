#!/bin/sh
set -eu

if [ "${CODEX_INSPECTOR_RUN_LIVE_PROOFS:-}" != "1" ]; then
  printf 'Set CODEX_INSPECTOR_RUN_LIVE_PROOFS=1 to launch a capacity-consuming disposable Codex task.\n' >&2
  exit 2
fi

source_codex_home="${CODEX_HOME:-${HOME}/.codex}"
if [ ! -f "${source_codex_home}/auth.json" ]; then
  printf 'A logged-in Codex home is required for the isolated live proof.\n' >&2
  exit 2
fi

isolated_home="$(mktemp -d "${TMPDIR:-/tmp}/codex-inspector-review-home.XXXXXX")"
chmod 700 "${isolated_home}"
trap 'rm -rf "${isolated_home}"' EXIT HUP INT TERM
cp "${source_codex_home}/auth.json" "${isolated_home}/auth.json"
chmod 600 "${isolated_home}/auth.json"

CODEX_HOME="${isolated_home}" codex plugin marketplace add "$(pwd)/fixtures/plugin-marketplace" >/dev/null
CODEX_HOME="${isolated_home}" codex plugin add codex-inspector@codex-inspector-phase0 --json >/dev/null

proof_dir="${isolated_home}/review"
mkdir -p "${proof_dir}"
cp schemas/reviews/report.schema.json "${proof_dir}/report.schema.json"
printf '%s\n' '{"schemaVersion":"inspector.review/v1","reviewId":"live-proof","createdAt":"2026-07-18T00:00:00Z","datasetEpoch":"fake-epoch","indexRevision":1,"scope":{"kind":"single_session","rootSessionId":"fake-root","descendantSessionIds":[],"appliedRevision":1},"includedSessionIds":["fake-root"],"includedTurnIds":["fake-turn"],"sources":[{"sourceId":"fake-source","sourceKind":"active_rollout","locator":"/fake/root.jsonl","fingerprint":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}],"aggregateMetrics":{"recorded_tokens":{"formulaVersion":1,"fidelity":"exact","value":0},"recorded_tokens_by_kind":{"formulaVersion":1,"fidelity":"exact","value":{"user_root_direct":0,"descendant":0,"inspector_review":0,"other_orphan":0}},"recorded_tokens_over_time":{"formulaVersion":1,"fidelity":"exact","value":[]},"token_composition":{"formulaVersion":1,"fidelity":"exact","value":{"uncached_input":0,"cached_input":0,"visible_output":0,"reasoning_output":0,"residual":0}},"top_root_sessions_by_tokens":{"formulaVersion":1,"fidelity":"exact","value":[]},"latest_capacity_observation":{"formulaVersion":1,"fidelity":"unavailable","value":null},"capacity_drawdown":{"formulaVersion":1,"fidelity":"unavailable","value":[]}},"coverageGaps":[],"evidenceRules":{"citationShape":"manifest_evidence_id","treatAsUntrusted":true},"rubric":["task_framing_and_steering","execution_efficiency","delegation_and_workflow","reusable_leverage"],"model":"configured-default","reasoning":"configured-default","evidence":[],"reportDestination":"./review.json","reportSchema":"./report.schema.json","limits":{"maxFindings":5,"maxReportBytes":1048576}}' > "${proof_dir}/manifest.json"

# The dollar-prefixed skill identifier is literal prompt text.
# shellcheck disable=SC2016
prompt='Use $codex-inspector:review-session. This is a disposable contract proof with fake evidence. Read ./manifest.json and write a schema-valid ./review.json with zero findings. Do not inspect or modify any other project.'
event_stream="${isolated_home}/temporary-exec-stream.jsonl"
CODEX_HOME="${isolated_home}" codex exec --json --sandbox workspace-write --skip-git-repo-check -C "${proof_dir}" "${prompt}" </dev/null > "${event_stream}"

thread_id="$(jq -er 'select(.type == "thread.started") | .thread_id | strings' "${event_stream}" | head -n 1)"
GOCACHE="${TMPDIR:-/tmp}/codex-inspector-go-cache" go run ./tools/phase0/reviewvalidate --schema "${proof_dir}/report.schema.json" --document "${proof_dir}/review.json" >/dev/null

resume_stream="${isolated_home}/temporary-resume-stream.jsonl"
CODEX_HOME="${isolated_home}" codex exec resume --json --skip-git-repo-check "${thread_id}" 'Reply with exactly: resume-proof-ok' </dev/null > "${resume_stream}"
jq -e --arg id "${thread_id}" 'select(.type == "thread.started" and .thread_id == $id)' "${resume_stream}" >/dev/null
jq -e 'select(.type == "item.completed" and .item.type == "agent_message" and .item.text == "resume-proof-ok")' "${resume_stream}" >/dev/null

if [ "${CODEX_INSPECTOR_PRESERVE_FOR_PTY:-}" = "1" ]; then
  state_file="${TMPDIR:-/tmp}/codex-inspector-manual-resume-state"
  umask 077
  printf 'CODEX_HOME=%s\nTHREAD_ID=%s\n' "${isolated_home}" "${thread_id}" > "${state_file}"
  trap - EXIT HUP INT TERM
  printf 'manual_resume_state=%s exec_resume=proved report_schema=proved\n' "${state_file}"
  exit 0
fi

/usr/bin/open -Ra Codex
/usr/bin/open -g "codex://threads/${thread_id}"

printf 'isolated_plugin_skill=proved persisted_exec=proved thread_started=proved report_schema=proved exec_resume=proved deep_link_invoked=proved visual_confirmation=not_claimed codex_resume=requires_manual_pty\n'
