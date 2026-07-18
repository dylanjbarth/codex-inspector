---
name: setup
description: Install and verify the Codex Inspector macOS CLI.
---

Confirm the machine is macOS arm64. Download `codex-inspector-darwin-arm64`
and `codex-inspector-darwin-arm64.sha256` from
`https://github.com/dylanjbarth/codex-inspector/releases/download/v0.1.0/`.
Run `shasum -a 256 -c codex-inspector-darwin-arm64.sha256`, then install the
verified binary as `codex-inspector` in a user-writable directory already on
`PATH`. Do not modify a shell profile. Run `codex-inspector doctor`, resolve
any reported hook-trust prompt in Codex, then run `codex-inspector open` and
`codex-inspector sync --background`. Never skip checksum verification.
