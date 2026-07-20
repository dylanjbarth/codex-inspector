# Codex Inspector

Codex Inspector is a local-first dashboard for understanding Codex token use,
session context, and effectiveness reviews. The current demo release supports
**macOS on Apple Silicon (arm64)** with the tested Codex source format. It does
not currently support Intel macOS, Linux, or Windows.

## Install

The user-install source of truth is the published
[`v0.1.1` GitHub Release](https://github.com/dylanjbarth/codex-inspector/releases/tag/v0.1.1).
Source builds are for development and are not the supported clean-install path.

### Automated CLI install

The installer checks for macOS arm64, requires its destination to already be
on `PATH`, downloads the `v0.1.1` CLI and checksum over HTTPS, verifies the
checksum, and installs only the CLI. The default destination is
`~/.local/bin`. It does not use `sudo`, alter shell profiles, install plugin
hooks, or read Codex data.

For the safer inspect-first flow:

```sh
curl -fsSL https://raw.githubusercontent.com/dylanjbarth/codex-inspector/main/install.sh \
  -o install-codex-inspector.sh
less install-codex-inspector.sh
sh install-codex-inspector.sh
rm install-codex-inspector.sh
```

Or run it directly after deciding that you trust it:

```sh
curl -fsSL https://raw.githubusercontent.com/dylanjbarth/codex-inspector/main/install.sh | sh
```

Set an absolute, user-writable destination when `~/.local/bin` is not suitable:

```sh
CODEX_INSPECTOR_INSTALL_DIR="$HOME/bin" sh install-codex-inspector.sh
```

The installer does not edit `PATH`. If the selected directory is absent from
`PATH`, it stops before downloading anything. Add the directory using your
preferred shell configuration, open a new terminal, and rerun the installer—or
select an existing absolute, user-writable directory already on `PATH`.

### Install the Codex plugin

The CLI installer intentionally does not duplicate the plugin-owned hooks.
Install the matching plugin from its pinned marketplace release:

```sh
codex plugin marketplace add "dylanjbarth/codex-inspector@v0.1.1"
codex plugin add codex-inspector@codex-inspector-development
```

Start Codex, choose **Review hooks**, confirm that all seven Inspector hooks
invoke `inspector-hook.sh`, and choose **Trust all and continue**.

Finish setup and open the dashboard:

```sh
codex-inspector doctor
codex-inspector open
```

`doctor` should report every check as `ok`. `open` starts or reuses the local
server, launches the dashboard, and automatically starts indexing in the
background when the server is new. Check that work with:

```sh
codex-inspector status
```

To explicitly block until a finite indexing pass completes and receive its
final JSON counts, run `codex-inspector sync --wait`. To queue another pass on
an already-running server without blocking, run `codex-inspector sync
--background`.

### Manual CLI install

If you prefer not to run an installer, download and verify the release assets
yourself. This copyable block fails before downloading unless `~/.local/bin` is
on `PATH`, stops before installation on any download or checksum error, and
always removes its private temporary directory:

```sh
(
  set -eu
  install_dir="$HOME/.local/bin"
  case ":$PATH:" in
    *:"$install_dir":*) ;;
    *) printf '%s\n' "$install_dir is not on PATH; add it and rerun" >&2; exit 1 ;;
  esac
  mkdir -p "$install_dir"
  work_dir=$(mktemp -d "${TMPDIR:-/tmp}/codex-inspector-install.XXXXXX")
  trap 'rm -rf -- "$work_dir"' EXIT HUP INT TERM
  chmod 700 "$work_dir"
  curl -fL --proto '=https' --tlsv1.2 \
    -o "$work_dir/codex-inspector-darwin-arm64" \
    https://github.com/dylanjbarth/codex-inspector/releases/download/v0.1.1/codex-inspector-darwin-arm64
  curl -fL --proto '=https' --tlsv1.2 \
    -o "$work_dir/codex-inspector-darwin-arm64.sha256" \
    https://github.com/dylanjbarth/codex-inspector/releases/download/v0.1.1/codex-inspector-darwin-arm64.sha256
  (cd "$work_dir" && shasum -a 256 -c codex-inspector-darwin-arm64.sha256)
  install -m 0755 "$work_dir/codex-inspector-darwin-arm64" \
    "$install_dir/codex-inspector"
)
```

Continue with the plugin commands above. Never install a binary that fails
checksum verification.

### Ask Codex to install it

Copy this prompt into Codex if you want it to guide the setup. It keeps each
mutation visible and requires confirmation:

> Install Codex Inspector from the official `dylanjbarth/codex-inspector`
> GitHub Release `v0.1.1`. First confirm this machine is macOS arm64 and explain
> what you will change. Ask for my confirmation before downloading or installing
> anything. Download the published CLI binary and checksum over HTTPS, verify
> the checksum, and install only the CLI into a user-writable directory already
> on `PATH` (prefer `~/.local/bin`). Do not use sudo or edit my shell profile.
> Then ask before installing the pinned marketplace
> `dylanjbarth/codex-inspector@v0.1.1` and plugin
> `codex-inspector@codex-inspector-development`. Do not copy credentials or
> inspect my Codex history. Tell me how to review and trust the seven plugin
> hooks myself. Finally run `codex-inspector doctor`; if healthy, run
> `codex-inspector open`, which starts indexing in the background, and then run
> `codex-inspector status`. Stop on any checksum,
> platform, compatibility, or trust failure and report it without bypassing it.

## Try the demo

Follow the [demo runbook](docs/demo-runbook.md) for the Token & Capacity,
Context Inspector, and Effectiveness Review flow. The
[Phase 6 verification guide](docs/phase6-verification.md) describes the
release evidence and deeper test commands. Exact compatibility details are in
the [support matrix](docs/contracts/support-matrix.md).

### Analytical skills

The plugin includes source-backed workflows for data-quality assessment,
dashboard use, report building, reusable data context, KPI design, analytical
validation, and quantitative visualization. These skills use Inspector's
versioned metrics, coverage, provenance, and frozen Review evidence rather than
recalculating results from raw Codex files:

- `$codex-inspector:analyze-data-quality`
- `$codex-inspector:build-dashboard`
- `$codex-inspector:build-report`
- `$codex-inspector:create-data-context`
- `$codex-inspector:design-kpis`
- `$codex-inspector:validate-data`
- `$codex-inspector:visualize-data`

## Privacy

Inspector runs locally and does not upload its index or source logs merely by
opening the dashboard. Evidence views intentionally display exact local
prompts, source code, tool arguments, and results, which may contain secrets or
personal data. Treat screen sharing, screenshots, and copied evidence as
sensitive. Starting an Effectiveness Review is a separate model boundary and
may send evidence to the user's configured Codex model service.
