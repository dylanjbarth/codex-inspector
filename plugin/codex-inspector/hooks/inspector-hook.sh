#!/bin/sh
set -u

# PLUGIN_DATA is intentionally diagnostic-only. The CLI exclusively owns
# CODEX_INSPECTOR_HOME and its queue.
record_diagnostic() {
  code="$1"
  severity="$2"
  [ -n "${PLUGIN_DATA:-}" ] || return 0
  umask 077
  mkdir -p "${PLUGIN_DATA}" 2>/dev/null || return 0
  chmod 700 "${PLUGIN_DATA}" 2>/dev/null || return 0
  final="${PLUGIN_DATA}/hook-diagnostic-${code}.json"
  if [ -f "${final}" ] && find "${final}" -mmin -1 -print 2>/dev/null | grep -q .; then
    return 0
  fi
  tmp="${PLUGIN_DATA}/.hook-diagnostic-${code}.$$.tmp"
  observed_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  printf '{"schemaVersion":"inspector.plugin-diagnostic/v1","code":"%s","severity":"%s","count":1,"lastObservedAt":"%s"}\n' "${code}" "${severity}" "${observed_at}" > "${tmp}" 2>/dev/null || return 0
  chmod 600 "${tmp}" 2>/dev/null || { rm -f "${tmp}" 2>/dev/null; return 0; }
  mv -f "${tmp}" "${final}" 2>/dev/null || rm -f "${tmp}" 2>/dev/null
}

if ! command -v codex-inspector >/dev/null 2>&1; then
  record_diagnostic missing_cli warning
  exit 0
fi
[ -n "${PLUGIN_DATA:-}" ] && rm -f "${PLUGIN_DATA}/hook-diagnostic-missing_cli.json" 2>/dev/null || true

if ! codex-inspector version 2>/dev/null | grep -q '^codex-inspector 0\.1\.[0-9][0-9]* (protocol 1, index schema 1)$'; then
  record_diagnostic protocol_mismatch error
  exit 0
fi
[ -n "${PLUGIN_DATA:-}" ] && rm -f "${PLUGIN_DATA}/hook-diagnostic-protocol_mismatch.json" 2>/dev/null || true

codex-inspector _hook --protocol 1 >/dev/null 2>&1 || exit 0
exit 0
