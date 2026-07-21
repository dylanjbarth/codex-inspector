#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)
cd "${repo_root}"

node_version=$(node --version)
node_major=$(printf '%s' "${node_version}" | sed -E 's/^v([0-9]+).*/\1/')
case "${node_major}" in ''|*[!0-9]*) exit 1 ;; esac
test "${node_major}" -ge 20

cli_version=$(pnpm exec playwright-cli --version)
test "${cli_version}" = "0.1.17"
runtime_version=$(node -e "const {createRequire}=require('node:module'); const r=createRequire(require.resolve('@playwright/cli/package.json')); process.stdout.write(r('playwright-core/package.json').version)")
test "${runtime_version}" = "1.62.0-alpha-1783623505000"

case "${1:-}" in
  '') pnpm exec playwright-cli install-browser chromium ;;
  --check) ;;
  *) echo 'usage: browser-prerequisite.sh [--check]' >&2; exit 2 ;;
esac

browser_path=$(node -e "const {createRequire}=require('node:module'); const r=createRequire(require.resolve('@playwright/cli/package.json')); process.stdout.write(r('playwright-core').chromium.executablePath())")
test -x "${browser_path}"
browser_version=$("${browser_path}" --version | sed 's/[[:space:]]*$//')
test "${browser_version}" = "Google Chrome for Testing 151.0.7922.10"

printf 'phase6_browser_prerequisite=passed node=%s playwright_cli=%s playwright_runtime=%s browser="%s" chromium_revision=1232\n' \
  "${node_version}" "${cli_version}" "${runtime_version}" "${browser_version}"
