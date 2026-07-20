---
name: setup
description: Install and verify the Codex Inspector macOS CLI.
---

Explain the planned changes and obtain the user's confirmation before
downloading or installing anything. Confirm the machine is macOS arm64. Use
the auditable installer from
`https://raw.githubusercontent.com/dylanjbarth/codex-inspector/main/install.sh`
or download `codex-inspector-darwin-arm64` and its `.sha256` file directly from
the official `v0.1.1` GitHub Release. Never skip checksum verification.

Install only the verified CLI into a user-writable directory already on
`PATH`, preferably `~/.local/bin`. Do not use `sudo`, modify a shell profile,
copy credentials, inspect Codex history, or install a second copy of the
plugin hooks. If the matching plugin is not installed, obtain confirmation
before running:

```sh
codex plugin marketplace add "dylanjbarth/codex-inspector@v0.1.1"
codex plugin add codex-inspector@codex-inspector-development
```

Tell the user to review and manually trust all seven Inspector hooks in Codex.
Then run `codex-inspector doctor`; stop and report any platform, checksum,
compatibility, or trust failure rather than bypassing it. Once healthy, run
`codex-inspector open`; a new server starts indexing in the background. Run
`codex-inspector status` to check that work. Use `codex-inspector sync --wait`
only when the user explicitly wants to block for a finite pass and its final
JSON counts.
