#!/bin/sh
set -eu
pnpm --filter @codex-inspector/web build
test -f web/dist/index.html
test -f web/dist/assets/index.js
test -f web/dist/assets/index.css
cp web/dist/index.html internal/server/assets/index.html
cp web/dist/assets/index.js internal/server/assets/assets/index.js
cp web/dist/assets/index.css internal/server/assets/assets/index.css
