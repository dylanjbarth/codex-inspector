#!/bin/sh
set -eu

codex_version="$(codex --version 2>/dev/null | tail -n 1)"
go_version="$(go version)"
machine_arch="$(uname -m)"
os_version="$(sw_vers -productVersion)"
codex_root="${CODEX_HOME:-${HOME}/.codex}"

active_count=0
if [ -d "${codex_root}/sessions" ]; then
  active_count="$(rg --files "${codex_root}/sessions" -g '*.jsonl' | wc -l | tr -d ' ')"
fi
archive_state="absent"
if [ -d "${codex_root}/archived_sessions" ]; then
  archive_state="present"
fi
session_index_state="absent"
if [ -f "${codex_root}/session_index.jsonl" ]; then
  session_index_state="present"
fi

printf 'codex=%s\n' "${codex_version}"
printf 'go=%s\n' "${go_version}"
printf 'os=macOS-%s architecture=%s\n' "${os_version}" "${machine_arch}"
printf 'active_rollout_count=%s archived_directory=%s session_index=%s\n' "${active_count}" "${archive_state}" "${session_index_state}"
printf 'supported_adapter=rollout-jsonl/codex-exact-cohorts/v2\n'
