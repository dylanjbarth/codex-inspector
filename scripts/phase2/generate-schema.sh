#!/bin/sh
set -eu

repo=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
source_file="$repo/docs/contracts/schema.sql"
generated="$repo/internal/storage/migrations/001_schema.sql"

if [ "${1:-}" = "--check" ]; then
  cmp -s "$source_file" "$generated" || {
    echo "generated Phase 2 schema is stale" >&2
    exit 1
  }
  exit 0
fi

mkdir -p "$(dirname -- "$generated")"
cp "$source_file" "$generated"
