#!/bin/sh
set -eu

generator='openapi-typescript@7.10.1'
source_file='schemas/internal-api.openapi.json'
output_file='web/src/generated/internal-api.ts'

if [ "${1:-}" = "--check" ]; then
  candidate="$(mktemp "${TMPDIR:-/tmp}/codex-inspector-api.XXXXXX.ts")"
  trap 'rm -f "${candidate}"' EXIT HUP INT TERM
  pnpm dlx "${generator}" "${source_file}" -o "${candidate}" >/dev/null
  cmp -s "${candidate}" "${output_file}" || {
    printf 'generated TypeScript contract is stale\n' >&2
    exit 1
  }
  printf 'generated_contract=current generator=%s\n' "${generator}"
  exit 0
fi

mkdir -p "$(dirname "${output_file}")"
pnpm dlx "${generator}" "${source_file}" -o "${output_file}"
