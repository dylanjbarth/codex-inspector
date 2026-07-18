#!/bin/sh
set -eu

if [ "${CODEX_INSPECTOR_RUN_LIVE_PROOFS:-}" != "1" ]; then
  printf 'Set CODEX_INSPECTOR_RUN_LIVE_PROOFS=1 to mutate an isolated temporary Codex home.\n' >&2
  exit 2
fi

isolated_home="$(mktemp -d "${TMPDIR:-/tmp}/codex-inspector-plugin-proof.XXXXXX")"
trap 'rm -rf "${isolated_home}"' EXIT HUP INT TERM
CODEX_HOME="${isolated_home}" codex plugin marketplace add "$(pwd)/fixtures/plugin-marketplace"
CODEX_HOME="${isolated_home}" codex plugin add codex-inspector@codex-inspector-phase0 --json >/dev/null
CODEX_HOME="${isolated_home}" codex plugin list --json | jq -e '.installed[] | select(.name == "codex-inspector" and .enabled == true)' >/dev/null

plugin_data="${isolated_home}/plugin-data"
PLUGIN_ROOT="$(pwd)/fixtures/plugin-marketplace/plugin" PLUGIN_DATA="${plugin_data}" fixtures/plugin-marketplace/plugin/hooks/probe.sh </dev/null
grep -q '^plugin_root=set$' "${plugin_data}/phase0-environment-proof.txt"
grep -q '^plugin_data=set$' "${plugin_data}/phase0-environment-proof.txt"

missing_cli_data="${isolated_home}/missing-cli-data"
for hook_fixture in fixtures/hooks/session-start.json fixtures/hooks/user-prompt-submit.json fixtures/hooks/pre-compact.json fixtures/hooks/post-compact.json fixtures/hooks/subagent-start.json fixtures/hooks/subagent-stop.json fixtures/hooks/stop.json; do
  PATH="/usr/bin:/bin" PLUGIN_ROOT="$(pwd)/fixtures/plugin-marketplace/plugin" PLUGIN_DATA="${missing_cli_data}" fixtures/plugin-marketplace/plugin/hooks/probe.sh < "${hook_fixture}"
done
test ! -e "${isolated_home}/queue"

printf 'marketplace_install=proved plugin_enabled=proved hook_environment_contract=proved seven_hook_definitions=proved missing_cli_nonblocking=proved hook_trust=requires_interactive_hash_acceptance\n'
