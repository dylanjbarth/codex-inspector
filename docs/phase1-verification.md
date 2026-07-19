# Phase 1 verification

Phase 1 supplies the plugin foundation only. Indexing and SQLite fact writes
begin in Phase 2, so `sync` reports an honest accepted empty state.

Run from a clean checkout on the frozen macOS arm64 demo target:

```text
pnpm install --frozen-lockfile
pnpm typecheck
pnpm lint
pnpm test
scripts/phase1/build-web.sh
GOCACHE="${TMPDIR}/codex-inspector-go-cache" GOPROXY=off GOSUMDB=off go test -count=1 ./...
GOCACHE="${TMPDIR}/codex-inspector-go-cache" GOPROXY=off GOSUMDB=off go test -race -count=1 ./...
GOCACHE="${TMPDIR}/codex-inspector-go-cache" GOPROXY=off GOSUMDB=off go vet ./...
GOCACHE="${TMPDIR}/codex-inspector-go-cache" GOPROXY=off GOSUMDB=off go build ./...
shellcheck scripts/phase1/*.sh plugin/codex-inspector/hooks/inspector-hook.sh
scripts/phase0/generate-internal-api.sh --check
scripts/phase0/generate-fact-golden.sh --check
scripts/phase1/package-macos.sh dist
(cd dist && shasum -a 256 -c codex-inspector-darwin-arm64.sha256)
GOCACHE="${TMPDIR}/codex-inspector-go-cache" go run ./tools/phase1/hookbench plugin/codex-inspector/hooks/inspector-hook.sh dist/codex-inspector-darwin-arm64 fixtures/hooks/stop.json
scripts/phase1/prove-clean-install.sh
```

The clean-install script uses isolated `CODEX_HOME` and
`CODEX_INSPECTOR_HOME` directories, installs from the development marketplace,
verifies the release-shaped macOS artifact, exercises missing, incompatible,
and available CLI hooks, verifies private payload-free `PLUGIN_DATA`
diagnostics through `doctor`, starts and reuses the process, loads the
dashboard through a real HTTP client, performs the frozen one-time
token/instance/protocol exchange, and observes idle shutdown. It never reads
the developer's Codex corpus.

The frozen `v0.1.x` plugin compatibility probe accepts the exact released CLI
shape `codex-inspector 0.1.x (protocol 1, index schema 2)`. A protocol or index
schema mismatch is non-blocking, records only the private payload-free
`protocol_mismatch` diagnostic, and does not invoke `_hook`. The clean-install
proof explicitly rejects the former index schema 1 output before proving that
the packaged schema 2 CLI creates marker-only queue state.

The hook benchmark times the registered plugin shell shim, including its CLI
version probe and second CLI process for marker creation. Before timing, it
also exercises the shim's missing-CLI diagnostic branch. A direct `_hook`
microbenchmark is only an optional breakdown and is not acceptance evidence.

Hook trust is deliberately manual. In the isolated home, start Codex, select
**Review hooks**, verify all seven definitions invoke the plugin-owned
`inspector-hook.sh`, then select **Trust all and continue**. Do not use the
hook-trust bypass flag. Run `codex-inspector doctor` in that same environment
and confirm all checks are `ok`. Finally run `codex-inspector open`, confirm the
browser fragment disappears after load and the empty state plus debugging tray
render, close the tab, and observe `codex-inspector status` report stopped after
120 seconds. Remove the isolated home after the proof.

`doctor` and server status obtain the exact installed plugin identity and the
seven current canonical hook hashes/trust states from Codex's plugin inventory
and `hooks/list` app-server method. Handwritten `trusted_hash` text is not
accepted as evidence. Changed, missing, untrusted, and unsupported hook states
remain explicit setup or incompatibility failures.

GitHub Releases for `v0.1.x` publish exactly
`codex-inspector-darwin-arm64` and its `.sha256` file. The setup skill pins the
matching tag and refuses installation until `shasum -a 256 -c` succeeds.
