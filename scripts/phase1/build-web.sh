#!/bin/sh
set -eu
pnpm --filter @codex-inspector/web build
test -f web/dist/index.html
test -f web/dist/assets/index.js
test -f web/dist/assets/index.css
test -f web/dist/assets/codex-inspector-logo.png
test -f web/dist/favicon-16.png
test -f web/dist/favicon-32.png
test -f web/dist/apple-touch-icon.png
cp web/dist/index.html internal/server/assets/index.html
cp web/dist/assets/index.js internal/server/assets/assets/index.js
cp web/dist/assets/index.css internal/server/assets/assets/index.css
cp web/dist/assets/codex-inspector-logo.png internal/server/assets/assets/codex-inspector-logo.png
cp web/dist/favicon-16.png internal/server/assets/favicon-16.png
cp web/dist/favicon-32.png internal/server/assets/favicon-32.png
cp web/dist/apple-touch-icon.png internal/server/assets/apple-touch-icon.png
