#!/bin/sh
set -eu

destination=fixtures/synthetic/expected-facts.json
temporary="$(mktemp "${TMPDIR:-/tmp}/codex-inspector-fact-golden.XXXXXX")"
trap 'rm -f "${temporary}"' EXIT HUP INT TERM

GOCACHE="${GOCACHE:-${TMPDIR:-/tmp}/codex-inspector-go-cache}" go run ./tools/phase0/factgolden > "${temporary}"

if [ "${1:-}" = "--check" ]; then
  if ! cmp -s "${temporary}" "${destination}"; then
    printf '%s\n' 'generated_fact_golden=stale' >&2
    exit 1
  fi
  printf '%s\n' 'generated_fact_golden=current'
  exit 0
fi

if [ "$#" -ne 0 ]; then
  printf 'usage: %s [--check]\n' "$0" >&2
  exit 2
fi

mv "${temporary}" "${destination}"
trap - EXIT HUP INT TERM
printf '%s\n' 'generated_fact_golden=updated'
