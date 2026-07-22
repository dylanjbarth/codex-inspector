---
name: open-dashboard
description: Set up Codex Inspector when needed, verify it, and open the local dashboard.
---

Check whether `codex-inspector` is available on `PATH`.

If the CLI is missing, explain the planned changes and obtain the user's
confirmation before downloading or installing anything. Confirm the machine is
macOS arm64. Use the auditable installer from
`https://raw.githubusercontent.com/dylanjbarth/codex-inspector/main/install.sh`
or download `codex-inspector-darwin-arm64` and its `.sha256` file directly from
the official `v0.1.2` GitHub Release. Never skip checksum verification.

Install only the verified CLI into a user-writable directory already on
`PATH`, preferably `~/.local/bin` when it is already on `PATH`. Do not use
`sudo`, modify a shell profile, copy credentials, inspect Codex history during
installation, or install another copy of the plugin or its hooks.

Run `codex-inspector doctor` after installation or when the CLI was already
available. Stop and report any platform, compatibility, or hook-trust failure
rather than bypassing it. Do not include local source paths or payloads in the
report. If the hooks are untrusted, ask the user to review and manually trust
all seven Inspector hooks in Codex.

Once `doctor` is healthy, run `codex-inspector open`; a new server starts
indexing in the background. Run `codex-inspector status` to report that work.
Use `codex-inspector sync --wait` only when the user explicitly wants to block
for a finite pass and its final JSON counts.
