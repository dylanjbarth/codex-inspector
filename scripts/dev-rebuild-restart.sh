#!/bin/sh
set -eu

unset CDPATH
repo_root=$(cd -- "$(dirname -- "$0")/.." && pwd)
cd "${repo_root}"

for required_command in go pnpm; do
  if ! command -v "${required_command}" >/dev/null 2>&1; then
    printf 'codex-inspector dev: required command not found: %s\n' "${required_command}" >&2
    exit 1
  fi
done

install_dir=${CODEX_INSPECTOR_DEV_GOBIN:-}
if [ -z "${install_dir}" ]; then
  active_cli=$(command -v codex-inspector 2>/dev/null || true)
  case "${active_cli}" in
    */*) install_dir=$(dirname -- "${active_cli}") ;;
  esac
fi
if [ -z "${install_dir}" ]; then
  install_dir=$(go env GOBIN)
fi
if [ -z "${install_dir}" ]; then
  go_path=$(go env GOPATH)
  install_dir=${go_path%%:*}/bin
fi

case "${install_dir}" in
  /*) ;;
  *)
    printf 'codex-inspector dev: install directory must be absolute: %s\n' "${install_dir}" >&2
    exit 1
    ;;
esac

mkdir -p "${install_dir}"
if [ ! -w "${install_dir}" ]; then
  printf 'codex-inspector dev: install directory is not writable: %s\n' "${install_dir}" >&2
  printf 'Set CODEX_INSPECTOR_DEV_GOBIN to a writable directory on PATH.\n' >&2
  exit 1
fi

installed_cli=${install_dir}/codex-inspector

printf 'Codex Inspector dev: building and embedding web assets...\n'
./scripts/phase1/build-web.sh

printf 'Codex Inspector dev: installing CLI at %s...\n' "${installed_cli}"
GOBIN="${install_dir}" go install ./cmd/codex-inspector

printf 'Codex Inspector dev: stopping the current local server...\n'
"${installed_cli}" stop

printf 'Codex Inspector dev: starting the rebuilt local server...\n'
"${installed_cli}" open "$@"
