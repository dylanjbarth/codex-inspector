import React from 'react'
import { establishSession, type Status } from './api'

function doctorDiagnostics(status: Status) {
  const attention = status.process.cliCompatibility === 'supported' ? 'ok' : status.process.cliCompatibility
  return JSON.stringify({
    summary: 'Payload-safe Inspector compatibility and runtime checks. Run codex-inspector doctor for live command output.',
    checks: [
      { name: 'inspector_cli', status: 'ok', detail: status.process.cliVersion },
      { name: 'codex_host', status: attention, detail: `compatibility=${status.process.cliCompatibility}` },
      { name: 'source_format', status: status.index.state === 'failed' ? 'error' : 'ok', detail: status.index.lastErrorCode ?? status.coverage?.reason ?? 'supported or empty' },
      { name: 'data_home', status: 'ok', detail: 'initialized with user-only ownership before server start' },
      { name: 'plugin', status: status.process.pluginVersion ? attention : 'missing', detail: `version=${status.process.pluginVersion ?? 'not detected'} protocol=${status.process.pluginProtocolVersion ?? 'unknown'}` },
      { name: 'hook_trust', status: attention, detail: `hook=${status.hook.state} registered=${status.hook.registeredEvents.length}` },
      { name: 'server', status: status.process.state, detail: 'authenticated loopback process' },
    ],
    tray: {
      inspectorVersion: status.process.inspectorVersion, cliVersion: status.process.cliVersion, processState: status.process.state,
      pluginVersion: status.process.pluginVersion, pluginProtocolVersion: status.process.pluginProtocolVersion, cliCompatibility: status.process.cliCompatibility,
      sources: { total: status.index.sourceCount, supported: status.index.supportedSourceCount, unsupported: status.index.unsupportedSourceCount, pendingTail: status.index.pendingTailCount, processed: status.index.processedCount, queued: status.index.queuedCount, skipped: status.index.skippedCount, failed: status.index.failedCount, requiresRebuild: status.index.requiresRebuildCount },
      database: { bytes: status.index.databaseBytes, schemaVersion: status.index.schemaVersion },
      scan: { reverseBoundary: status.index.reverseScanBoundary, completedWatermark: status.index.completedWatermark },
      hooks: { state: status.hook.state, registeredEvents: status.hook.registeredEvents, lastMarker: status.hook.lastMarker, diagnostics: status.hook.diagnostics },
    },
  }, null, 2)
}

export function App() {
  const [status, setStatus] = React.useState<Status | null>(null)
  const [error, setError] = React.useState('')
  React.useEffect(() => {
    establishSession().then(setStatus).catch((e: Error) => setError(e.message))
    const heartbeat = setInterval(() => void fetch('/v1/heartbeat', { method: 'POST' }), 15_000)
    return () => clearInterval(heartbeat)
  }, [])
  if (error) return <main><h1>Codex Inspector</h1><section className="error"><h2>Unable to open Inspector</h2><p>{error}</p><code>codex-inspector doctor</code></section></main>
  if (!status) return <main><h1>Codex Inspector</h1><p>Establishing a secure local session…</p></main>
  const diagnostics = doctorDiagnostics(status)
  const setup = !status.process.pluginVersion || status.process.cliCompatibility === 'unknown'
  const incompatible = status.process.cliCompatibility === 'unsupported'
  const degraded = status.process.state === 'degraded' || status.index.state === 'failed' || status.hook.state === 'degraded'
  const heading = setup ? 'Finish Inspector setup' : incompatible ? 'Inspector versions are incompatible' : degraded ? 'Inspector needs attention' : 'Ready for your first sync'
  const message = setup ? 'Install and enable the matching Codex Inspector plugin, then review its seven hooks.' : incompatible ? 'The installed Codex host, CLI, plugin, or source format is outside the supported Phase 1 contract.' : degraded ? 'A compatibility or hook diagnostic requires attention before syncing.' : 'The secure Inspector shell is running. Session indexing begins in Phase 2.'
  return <main><header><div><span className="eyebrow">LOCAL OBSERVABILITY</span><h1>Codex Inspector</h1></div><span className="pill">Process {status.process.state}</span></header><section className={degraded || setup || incompatible ? 'error' : 'empty'}><h2>{heading}</h2><p>{message}</p>{degraded || setup || incompatible ? <code>codex-inspector doctor</code> : <button onClick={() => void fetch('/v1/sync', { method: 'POST', headers: { 'content-type': 'application/json' }, body: '{"mode":"background"}' })}>Check sync status</button>}</section><details><summary>Status &amp; debugging</summary><dl><dt>Inspector / CLI / process</dt><dd>{status.process.inspectorVersion} / {status.process.cliVersion} / {status.process.state}</dd><dt>Plugin / protocol</dt><dd>{status.process.pluginVersion ?? 'not detected'} / {status.process.pluginProtocolVersion ?? 'unknown'}</dd><dt>Codex host compatibility</dt><dd>{status.process.cliCompatibility}</dd><dt>Source format / data home</dt><dd>{status.index.state === 'failed' ? status.index.lastErrorCode ?? 'attention required' : 'supported or empty'} / initialized</dd><dt>Hook trust / server</dt><dd>{status.process.cliCompatibility === 'supported' ? 'compatible' : 'attention required'} / {status.process.state}</dd><dt>Index</dt><dd>{status.index.state}</dd><dt>Source inventory</dt><dd>{status.index.sourceCount} total · {status.index.supportedSourceCount} supported · {status.index.unsupportedSourceCount} unsupported · {status.index.pendingTailCount} pending tail</dd><dt>Source progress</dt><dd>{status.index.processedCount} processed · {status.index.queuedCount} queued · {status.index.skippedCount} skipped · {status.index.failedCount} failed · {status.index.requiresRebuildCount} rebuild</dd><dt>Queued markers</dt><dd>{status.index.queuedSessionChanges}</dd><dt>Reverse scan / watermark</dt><dd>{status.index.reverseScanBoundary ?? 'not started'} / {status.index.completedWatermark ?? 'none'}</dd><dt>Database / schema</dt><dd>{status.index.databaseBytes} bytes / {status.index.schemaVersion}</dd><dt>Registered hooks</dt><dd>{status.hook.registeredEvents.join(', ')}</dd><dt>Last marker</dt><dd>{status.hook.lastMarker ? `${status.hook.lastMarker.eventKind} at ${status.hook.lastMarker.observedAt}` : 'none'}</dd><dt>Hook diagnostics</dt><dd>{status.hook.diagnostics.length ? status.hook.diagnostics.map(d => `${d.code} ×${d.count}`).join(', ') : 'none'}</dd></dl><pre>{diagnostics}</pre><button onClick={() => void navigator.clipboard.writeText(diagnostics)}>Copy doctor diagnostics</button></details><aside>Sensitive-data notice: later evidence views may display exact local payloads.</aside></main>
}
