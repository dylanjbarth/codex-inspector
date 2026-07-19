#!/bin/sh
set -eu

profile_root=$(mktemp -d "${TMPDIR:-/tmp}/codex-inspector-phase6-profile.XXXXXX")
cleanup() { rm -rf -- "${profile_root}"; }
trap cleanup EXIT HUP INT TERM
chmod 700 "${profile_root}"
profile_binary="${profile_root}/corpusprofile"
GOCACHE="${GOCACHE:-${TMPDIR:-/tmp}/codex-inspector-phase6-gocache}" \
  go build -o "${profile_binary}" ./tools/phase6/corpusprofile

# The Go probe emits aggregate counts, durations, memory, and frozen limits
# only. /usr/bin/time writes resource counters to the disposable file.
CODEX_INSPECTOR_HOME="${profile_root}/inspector" \
  /usr/bin/time -l -o "${profile_root}/resources.txt" \
  "${profile_binary}"

awk '
  /maximum resident set size/ { print "max_rss_bytes=" $1 }
  /user time/ { print "user_seconds=" $1 }
  /system time/ { print "system_seconds=" $1 }
' "${profile_root}/resources.txt"
