#!/bin/sh
set -eu

release_version="v0.1.2"
repository="dylanjbarth/codex-inspector"
artifact="codex-inspector-darwin-arm64"
install_dir="${CODEX_INSPECTOR_INSTALL_DIR:-${HOME}/.local/bin}"
temp_dir=""
staged_path=""

cleanup() {
  if [ -n "${staged_path}" ] && [ -e "${staged_path}" ]; then
    rm -f -- "${staged_path}"
  fi
  if [ -n "${temp_dir}" ] && [ -d "${temp_dir}" ]; then
    rm -rf -- "${temp_dir}"
  fi
}
trap cleanup EXIT HUP INT TERM

fail() {
  printf 'codex-inspector installer: %s\n' "$1" >&2
  exit 1
}

[ "$(uname -s)" = "Darwin" ] || fail "only macOS is supported by this release"
[ "$(uname -m)" = "arm64" ] || fail "only Apple Silicon (arm64) is supported by this release"
command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v shasum >/dev/null 2>&1 || fail "shasum is required"
command -v install >/dev/null 2>&1 || fail "install is required"

case "${install_dir}" in
  /*) ;;
  *) fail "CODEX_INSPECTOR_INSTALL_DIR must be an absolute path" ;;
esac

case ":${PATH}:" in
  *:"${install_dir}":*) ;;
  *)
    fail "install destination is not on PATH: ${install_dir}; add it using your preferred shell configuration and start a new terminal, or rerun with CODEX_INSPECTOR_INSTALL_DIR set to an existing user-writable PATH directory"
    ;;
esac

if [ -e "${install_dir}" ]; then
  [ -d "${install_dir}" ] || fail "install destination is not a directory: ${install_dir}"
  [ -w "${install_dir}" ] || fail "install destination is not user-writable: ${install_dir}"
else
  writable_parent=${install_dir}
  while [ ! -e "${writable_parent}" ]; do
    next_parent=${writable_parent%/*}
    [ -n "${next_parent}" ] || next_parent=/
    [ "${next_parent}" != "${writable_parent}" ] || break
    writable_parent=${next_parent}
  done
  [ -d "${writable_parent}" ] && [ -w "${writable_parent}" ] ||
    fail "install destination cannot be created by this user: ${install_dir}"
fi

mkdir -p "${install_dir}"
[ -d "${install_dir}" ] || fail "install destination is not a directory: ${install_dir}"
[ -w "${install_dir}" ] || fail "install destination is not user-writable: ${install_dir}"
[ ! -d "${install_dir}/codex-inspector" ] ||
  fail "install target is an existing directory: ${install_dir}/codex-inspector"

umask 077
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/codex-inspector-install.XXXXXX") ||
  fail "could not create a private temporary directory"
chmod 700 "${temp_dir}"

base_url="https://github.com/${repository}/releases/download/${release_version}"
curl -fL --proto '=https' --tlsv1.2 \
  -o "${temp_dir}/${artifact}" "${base_url}/${artifact}"
curl -fL --proto '=https' --tlsv1.2 \
  -o "${temp_dir}/${artifact}.sha256" "${base_url}/${artifact}.sha256"

(
  cd "${temp_dir}"
  shasum -a 256 -c "${artifact}.sha256"
) || fail "published checksum verification failed; nothing was installed"

staged_path="${install_dir}/.codex-inspector.new.$$"
install -m 0755 "${temp_dir}/${artifact}" "${staged_path}"
mv -f -- "${staged_path}" "${install_dir}/codex-inspector"
staged_path=""

printf '\nInstalled Codex Inspector %s to:\n  %s/codex-inspector\n' \
  "${release_version}" "${install_dir}"

cat <<'NEXT_STEPS'

Next, install the matching Codex plugin (the CLI installer does not install hooks):
  codex plugin marketplace add "dylanjbarth/codex-inspector@v0.1.2"
  codex plugin add codex-inspector@codex-inspector-development

Start Codex and review and trust the seven Inspector hooks, then run:
  codex-inspector doctor
  codex-inspector open
  codex-inspector sync --wait
NEXT_STEPS
