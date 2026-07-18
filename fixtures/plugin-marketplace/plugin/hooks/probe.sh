#!/bin/sh
set -eu

if [ -z "${PLUGIN_ROOT:-}" ] || [ -z "${PLUGIN_DATA:-}" ]; then
  exit 0
fi
mkdir -p "${PLUGIN_DATA}"
umask 077
printf 'plugin_root=set\nplugin_data=set\n' > "${PLUGIN_DATA}/phase0-environment-proof.txt"

# This is the Phase 0 shape of the product shim: a missing CLI must never
# block Codex and must not create an Inspector work marker.
if ! command -v codex-inspector >/dev/null 2>&1; then
  exit 0
fi

codex-inspector _hook || exit 0
exit 0
