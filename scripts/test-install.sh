#!/bin/sh
set -eu

unset CDPATH
repo_root=$(cd -- "$(dirname -- "$0")/.." && pwd)
test_root=$(mktemp -d "${TMPDIR:-/tmp}/codex-inspector-installer-test.XXXXXX")
trap 'rm -rf -- "${test_root}"' EXIT HUP INT TERM

make_case() {
  case_name=$1
  case_root="${test_root}/${case_name}"
  mkdir -p "${case_root}/bin" "${case_root}/home" "${case_root}/tmp"
  printf 'profile must remain unchanged\n' >"${case_root}/home/.zshrc"

  cat >"${case_root}/bin/uname" <<'MOCK_UNAME'
#!/bin/sh
case "$1" in
  -s) printf '%s\n' "${TEST_UNAME_S:-Darwin}" ;;
  -m) printf '%s\n' "${TEST_UNAME_M:-arm64}" ;;
  *) exit 2 ;;
esac
MOCK_UNAME

cat >"${case_root}/bin/curl" <<'MOCK_CURL'
#!/bin/sh
output=""
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) output=$2; shift 2 ;;
    -*) shift ;;
    *) url=$1; shift ;;
  esac
done
[ -z "${TEST_CURL_MARKER:-}" ] || : >"${TEST_CURL_MARKER}"
case "${url}" in
  https://github.com/dylanjbarth/codex-inspector/releases/download/v0.1.1/codex-inspector-darwin-arm64)
    printf '#!/bin/sh\nprintf "installed fixture\\n"\n' >"${output}"
    [ "${TEST_CURL_FAIL:-0}" != 1 ] || exit 22
    ;;
  https://github.com/dylanjbarth/codex-inspector/releases/download/v0.1.1/codex-inspector-darwin-arm64.sha256)
    if [ "${TEST_BAD_CHECKSUM:-0}" = 1 ]; then
      printf '%064d  codex-inspector-darwin-arm64\n' 0 >"${output}"
    else
      digest=$(shasum -a 256 "$(dirname -- "${output}")/codex-inspector-darwin-arm64" | awk '{print $1}')
      printf '%s  codex-inspector-darwin-arm64\n' "${digest}" >"${output}"
    fi
    ;;
  *) printf 'unexpected URL: %s\n' "${url}" >&2; exit 3 ;;
esac
MOCK_CURL
  chmod 755 "${case_root}/bin/uname" "${case_root}/bin/curl"
}

assert_clean_temp() {
  case_root=$1
  if find "${case_root}/tmp" -mindepth 1 -print -quit | grep -q .; then
    printf 'temporary installer files were not cleaned up\n' >&2
    exit 1
  fi
}

make_case success
success_root="${test_root}/success"
success_destination="${success_root}/destination with spaces"
mkdir -p "${success_destination}"
printf '#!/bin/sh\nprintf "old fixture\\n"\n' >"${success_destination}/codex-inspector"
chmod 755 "${success_destination}/codex-inspector"
PATH="${success_root}/bin:${success_destination}:/usr/bin:/bin" \
HOME="${success_root}/home" \
TMPDIR="${success_root}/tmp" \
CODEX_INSPECTOR_INSTALL_DIR="${success_destination}" \
  sh <"${repo_root}/install.sh" >"${success_root}/stdout" 2>"${success_root}/stderr"
installed="${success_destination}/codex-inspector"
[ -x "${installed}" ]
[ "$(stat -f '%Lp' "${installed}")" = 755 ]
"${installed}" | grep -q '^installed fixture$'
grep -q 'codex plugin marketplace add.*v0.1.1' "${success_root}/stdout"
grep -q 'codex-inspector doctor' "${success_root}/stdout"
grep -q '^profile must remain unchanged$' "${success_root}/home/.zshrc"
[ ! -e "${success_root}/home/.codex" ]
assert_clean_temp "${success_root}"

make_case checksum_failure
checksum_root="${test_root}/checksum_failure"
mkdir -p "${checksum_root}/destination"
printf 'existing cli must survive\n' >"${checksum_root}/destination/codex-inspector"
if PATH="${checksum_root}/bin:${checksum_root}/destination:/usr/bin:/bin" \
  HOME="${checksum_root}/home" TMPDIR="${checksum_root}/tmp" \
  CODEX_INSPECTOR_INSTALL_DIR="${checksum_root}/destination" \
  TEST_BAD_CHECKSUM=1 sh "${repo_root}/install.sh" \
  >"${checksum_root}/stdout" 2>"${checksum_root}/stderr"; then
  printf 'bad checksum unexpectedly succeeded\n' >&2
  exit 1
fi
grep -q '^existing cli must survive$' "${checksum_root}/destination/codex-inspector"
grep -q 'checksum verification failed' "${checksum_root}/stderr"
assert_clean_temp "${checksum_root}"

make_case path_failure
path_root="${test_root}/path_failure"
if PATH="${path_root}/bin:/usr/bin:/bin" \
  HOME="${path_root}/home" TMPDIR="${path_root}/tmp" \
  CODEX_INSPECTOR_INSTALL_DIR="${path_root}/destination" \
  TEST_CURL_MARKER="${path_root}/curl-was-called" sh "${repo_root}/install.sh" \
  >"${path_root}/stdout" 2>"${path_root}/stderr"; then
  printf 'destination absent from PATH unexpectedly succeeded\n' >&2
  exit 1
fi
[ ! -e "${path_root}/curl-was-called" ]
[ ! -e "${path_root}/destination" ]
grep -q 'install destination is not on PATH' "${path_root}/stderr"
assert_clean_temp "${path_root}"

make_case interrupted_download
interrupted_root="${test_root}/interrupted_download"
mkdir -p "${interrupted_root}/destination"
printf 'existing cli must survive interruption\n' >"${interrupted_root}/destination/codex-inspector"
if PATH="${interrupted_root}/bin:${interrupted_root}/destination:/usr/bin:/bin" \
  HOME="${interrupted_root}/home" TMPDIR="${interrupted_root}/tmp" \
  CODEX_INSPECTOR_INSTALL_DIR="${interrupted_root}/destination" \
  TEST_CURL_FAIL=1 sh "${repo_root}/install.sh" \
  >"${interrupted_root}/stdout" 2>"${interrupted_root}/stderr"; then
  printf 'interrupted download unexpectedly succeeded\n' >&2
  exit 1
fi
grep -q '^existing cli must survive interruption$' "${interrupted_root}/destination/codex-inspector"
assert_clean_temp "${interrupted_root}"

make_case platform_failure
platform_root="${test_root}/platform_failure"
if PATH="${platform_root}/bin:${platform_root}/destination:/usr/bin:/bin" \
  HOME="${platform_root}/home" TMPDIR="${platform_root}/tmp" \
  CODEX_INSPECTOR_INSTALL_DIR="${platform_root}/destination" \
  TEST_UNAME_S=Linux sh "${repo_root}/install.sh" \
  >"${platform_root}/stdout" 2>"${platform_root}/stderr"; then
  printf 'unsupported platform unexpectedly succeeded\n' >&2
  exit 1
fi
[ ! -e "${platform_root}/destination/codex-inspector" ]
grep -q 'only macOS is supported' "${platform_root}/stderr"
assert_clean_temp "${platform_root}"

make_case architecture_failure
architecture_root="${test_root}/architecture_failure"
if PATH="${architecture_root}/bin:${architecture_root}/destination:/usr/bin:/bin" \
  HOME="${architecture_root}/home" TMPDIR="${architecture_root}/tmp" \
  CODEX_INSPECTOR_INSTALL_DIR="${architecture_root}/destination" \
  TEST_UNAME_M=x86_64 sh "${repo_root}/install.sh" \
  >"${architecture_root}/stdout" 2>"${architecture_root}/stderr"; then
  printf 'unsupported architecture unexpectedly succeeded\n' >&2
  exit 1
fi
[ ! -e "${architecture_root}/destination/codex-inspector" ]
grep -q 'only Apple Silicon' "${architecture_root}/stderr"
assert_clean_temp "${architecture_root}"

printf 'installer tests passed: stdin success/replacement, checksum preservation, PATH/platform gates, interrupted-download cleanup, and no profile/plugin mutation\n'
