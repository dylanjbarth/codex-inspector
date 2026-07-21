/* eslint-disable @typescript-eslint/no-explicit-any */

export const DEMO_MODE = import.meta.env.VITE_DEMO_MODE === 'true'

const epoch = 'sample-2026-07-22'
const revision = 12
const exact = { fidelity: 'exact', observed: 12, eligible: 12 }
const now = '2026-07-22T01:25:00Z'

const sessions = [
  {
    sessionId: 'sample-build-week', rootWorkUnitId: 'sample-build-week', purpose: 'user',
    title: 'Prepare the Build Week demo', project: '/Users/demo/codex-inspector',
    startedAt: '2026-07-21T15:02:00Z', latestCompleted: '2026-07-21T17:38:00Z', completedTurns: 14,
    descendantSessions: 3, matchCategories: ['root: user message'], matchSnippets: [{ category: 'root: user message', text: 'Build an interactive demo that makes agent delegation and evidence easy to understand.' }],
    directTokens: 428000, descendantTokens: 193000,
  },
  {
    sessionId: 'sample-search-fix', rootWorkUnitId: 'sample-search-fix', purpose: 'user',
    title: 'Fix session discovery for older IDs', project: '/Users/demo/codex-inspector',
    startedAt: '2026-07-21T11:20:00Z', latestCompleted: '2026-07-21T12:08:00Z', completedTurns: 7,
    descendantSessions: 1, matchCategories: ['root: user message'], matchSnippets: [{ category: 'root: user message', text: 'Find why older completed session IDs return no matches and add a regression test.' }],
    directTokens: 214000, descendantTokens: 76000,
  },
  {
    sessionId: 'sample-judge-skills', rootWorkUnitId: 'sample-judge-skills', purpose: 'user',
    title: 'Evaluate the project against Build Week criteria', project: '/Users/demo/codex-inspector',
    startedAt: '2026-07-20T18:10:00Z', latestCompleted: '2026-07-20T19:31:00Z', completedTurns: 9,
    descendantSessions: 2, matchCategories: ['root: user message'], matchSnippets: [{ category: 'root: user message', text: 'Run the AI and human judge simulations and compare readiness with the previous baseline.' }],
    directTokens: 305000, descendantTokens: 119000,
  },
]

const status = {
  schemaVersion: 2, datasetEpoch: epoch, appliedRevision: revision, coverage: exact,
  sourceHome: { path: '/sample/CODEX_HOME', resolution: 'environment' },
  inspectorHome: { path: '/sample/CODEX_INSPECTOR_HOME' },
  process: { state: 'ready', inspectorVersion: '0.1.1-demo', cliVersion: '0.1.1', cliCompatibility: 'supported', pluginVersion: '0.1.1', pluginProtocolVersion: 1, pid: 2026, startedAt: now },
  index: { state: 'current', schemaVersion: 2, datasetEpoch: epoch, appliedRevision: revision, databaseBytes: 14892032, sourceCount: 48, supportedSourceCount: 47, unsupportedSourceCount: 1, pendingTailCount: 0, queuedSessionChanges: 0, inventoriedCount: 48, processedCount: 47, remainingCount: 0, queuedCount: 0, skippedCount: 1, failedCount: 0, requiresRebuildCount: 0, diagnosticGroups: [], reverseScanBoundary: '2026-06-22T00:00:00Z', completedWatermark: now },
  hook: { state: 'healthy', registeredEvents: ['SessionStart','UserPromptSubmit','PreCompact','PostCompact','SubagentStart','SubagentStop','Stop'], lastMarker: null, diagnostics: [] },
}

const filterOptions = {
  projects: [{ value: 'codex-inspector', label: 'codex-inspector' }, { value: 'build-week', label: 'Build Week' }],
  models: ['gpt-5.6-sol', 'gpt-5.6-terra'], reasoningEfforts: ['high', 'xhigh'],
  contributionKinds: [
    { value: 'user_root_direct', label: 'Direct user-session work' },
    { value: 'descendant', label: 'Spawned agent work' },
    { value: 'inspector_review', label: 'Codex Inspector reviews' },
    { value: 'other_orphan', label: 'Other or unlinked session work' },
  ],
}

const meta = {
  formulaVersion: 1, fidelity: 'exact', coverage: exact,
  indexedCoverage: { indexedStart: '2026-06-22T00:00:00Z', indexedEnd: now, completedWatermark: now },
  timeBoundary: { requestedStart: null, requestedEnd: null, effectiveStart: '2026-06-22T00:00:00Z', effectiveEnd: now, timezone: 'UTC' }, exclusionReasons: {},
}

function metrics(request: any = {}) {
  const onlyDescendants = request.contributionKinds?.length === 1 && request.contributionKinds[0] === 'descendant'
  const total = onlyDescendants ? 388000 : 1482000
  const direct = onlyDescendants ? 0 : 947000
  const descendants = onlyDescendants ? 388000 : 388000
  const review = onlyDescendants ? 0 : 121000
  const other = onlyDescendants ? 0 : 26000
  const days = [
    ['2026-07-16T00:00:00Z', 118000, 26000], ['2026-07-17T00:00:00Z', 91000, 48000],
    ['2026-07-18T00:00:00Z', 126000, 55000], ['2026-07-19T00:00:00Z', 103000, 33000],
    ['2026-07-20T00:00:00Z', 165000, 62000], ['2026-07-21T00:00:00Z', 241000, 114000],
    ['2026-07-22T00:00:00Z', 103000, 50000],
  ]
  return { schemaVersion: 2, datasetEpoch: epoch, appliedRevision: revision, coverage: exact, results: [
    { ...meta, key: 'recorded_tokens', value: total },
    { ...meta, key: 'recorded_tokens_by_kind', value: { userRootDirect: direct, descendant: descendants, inspectorReview: review, otherOrphan: other } },
    { ...meta, key: 'recorded_tokens_over_time', value: days.map(([bucketStart, userRootDirect, descendant]) => ({ bucketStart, bucketEnd: new Date(Date.parse(String(bucketStart)) + 86400000).toISOString(), timezone: 'UTC', grain: 'day', byKind: { userRootDirect: onlyDescendants ? 0 : userRootDirect, descendant, inspectorReview: onlyDescendants ? 0 : 12000, otherOrphan: onlyDescendants ? 0 : 3000 } })) },
    { ...meta, key: 'token_composition', value: { uncachedInput: Math.round(total * .43), cachedInput: Math.round(total * .32), visibleOutput: Math.round(total * .17), reasoningOutput: Math.round(total * .08), residual: 0 } },
    { ...meta, key: 'top_root_sessions_by_tokens', value: sessions.map(item => ({ rootSessionId: item.sessionId, inclusiveTokens: item.directTokens + item.descendantTokens, directTokens: item.directTokens, descendantTokens: item.descendantTokens })) },
    { ...meta, key: 'latest_capacity_observation', value: [{ observedAt: now, limitId: 'codex', windowMinutes: 300, usedPercent: 38, remainingPercent: 62, resetsAt: '2026-07-22T05:00:00Z', stale: false }, { observedAt: now, limitId: 'codex', windowMinutes: 10080, usedPercent: 64, remainingPercent: 36, resetsAt: '2026-07-27T00:00:00Z', stale: false }] },
    { ...meta, key: 'capacity_drawdown', value: [{ limitId: 'codex', windowMinutes: 10080, resetBoundary: '2026-07-27T00:00:00Z', points: [
      { observedAt: '2026-07-16T00:00:00Z', usedPercent: 8 }, { observedAt: '2026-07-17T00:00:00Z', usedPercent: 17 }, { observedAt: '2026-07-18T00:00:00Z', usedPercent: 29 }, { observedAt: '2026-07-19T00:00:00Z', usedPercent: 36 }, { observedAt: '2026-07-20T00:00:00Z', usedPercent: 47 }, { observedAt: '2026-07-21T00:00:00Z', usedPercent: 59 }, { observedAt: now, usedPercent: 64 },
    ] }] },
  ] }
}

function sessionPage(query = '') {
  const needle = query.trim().toLowerCase()
  const items = sessions.filter(item => !needle || [item.title, item.project, ...item.matchSnippets.map(snippet => snippet.text)].join(' ').toLowerCase().includes(needle))
  return { schemaVersion: 2, datasetEpoch: epoch, appliedRevision: revision, coverage: { fidelity: 'exact', observed: items.length, eligible: sessions.length }, items }
}

function sessionMap(rootId: string) {
  const root = sessions.find(item => item.sessionId === rootId) ?? sessions[0]
  const childId = `${root.sessionId}-agent`
  const rootTurn = `${root.sessionId}-turn-1`
  return { schemaVersion: 2, datasetEpoch: epoch, appliedRevision: revision, coverage: exact, rootSessionId: root.sessionId,
    nodes: [{ sessionId: root.sessionId, kind: 'root', directTokens: root.directTokens, inclusiveTokens: root.directTokens + root.descendantTokens }, { sessionId: childId, kind: 'descendant', directTokens: root.descendantTokens, inclusiveTokens: root.descendantTokens, spawningTurnId: rootTurn }],
    edges: [{ parentSessionId: root.sessionId, childSessionId: childId, kind: 'spawned' }],
    rootTurns: [{ turnId: rootTurn, ordinal: 0, state: 'completed', startedAt: root.startedAt, completedAt: root.latestCompleted }],
    turns: [
      { turnId: rootTurn, sessionId: root.sessionId, sessionKind: 'root', promptPreview: root.matchSnippets[0].text, ordinal: 0, state: 'completed', startedAt: root.startedAt, completedAt: root.latestCompleted, directTokens: root.directTokens, inclusiveTokens: root.directTokens + root.descendantTokens, toolCount: 6, errorCount: root.sessionId === 'sample-search-fix' ? 1 : 0, compactionCount: 1 },
      { turnId: `${childId}-turn-1`, sessionId: childId, sessionKind: 'descendant', promptPreview: root.sessionId === 'sample-judge-skills' ? 'Run the human judge simulation and cite the strongest gaps.' : 'Inspect the relevant implementation and report evidence.', ordinal: 0, state: 'completed', startedAt: root.startedAt, completedAt: root.latestCompleted, directTokens: root.descendantTokens, inclusiveTokens: root.descendantTokens, toolCount: 3, errorCount: 0, compactionCount: 0 },
    ],
    spawnTopology: [{ parentSessionId: root.sessionId, childSessionId: childId, spawnTurnId: rootTurn, edgeKind: 'spawned', ordinal: 0 }],
  }
}

function ledger(turnId: string) {
  return { schemaVersion: 2, datasetEpoch: epoch, appliedRevision: revision, coverage: { fidelity: 'exact', observed: 7, eligible: 7 }, turnId, items: [
    { eventId: 'sample-user', evidenceId: 'sample-user', kind: 'message', observedAt: '2026-07-21T15:02:03Z', actor: 'user', family: 'message', messageRole: 'user' },
    { eventId: 'sample-token', evidenceId: 'sample-token', kind: 'token_count', observedAt: '2026-07-21T15:02:06Z', actor: 'runtime', family: 'runtime' },
    { eventId: 'sample-tool', evidenceId: 'sample-tool', kind: 'function_call', observedAt: '2026-07-21T15:02:09Z', actor: 'agent', family: 'tool', callId: 'sample-call', toolPhase: 'request', toolName: 'exec_command', toolFamily: 'shell', status: 'completed' },
    { eventId: 'sample-tool-result', evidenceId: 'sample-tool-result', kind: 'function_call_output', observedAt: '2026-07-21T15:02:10Z', actor: 'tool', family: 'tool', callId: 'sample-call', toolPhase: 'result', toolName: 'exec_command', toolFamily: 'shell', status: 'completed', exitCode: 0, durationMs: 1840 },
    { eventId: 'sample-agent', evidenceId: 'sample-agent', kind: 'message', observedAt: '2026-07-21T15:11:02Z', actor: 'agent', family: 'message', messageRole: 'assistant' },
    { eventId: 'sample-compacted', evidenceId: 'sample-compacted', kind: 'compacted', observedAt: '2026-07-21T16:01:01Z', actor: 'runtime', family: 'compaction' },
    { eventId: 'sample-response', evidenceId: 'sample-response', kind: 'message', observedAt: '2026-07-21T17:38:02Z', actor: 'agent', family: 'message', messageRole: 'assistant' },
  ] }
}

function evidence(evidenceId: string) {
  const records: Record<string, unknown> = {
    'sample-user': { timestamp: '2026-07-21T15:02:03Z', type: 'response_item', payload: { type: 'message', role: 'user', content: [{ type: 'input_text', text: 'Build an interactive demo that makes agent delegation and evidence easy to understand.' }] } },
    'sample-token': { timestamp: '2026-07-21T15:02:06Z', type: 'event_msg', payload: { type: 'token_count', info: { model_context_window: 258400, last_token_usage: { input_tokens: 18200, cached_input_tokens: 12100, output_tokens: 2100, reasoning_output_tokens: 960, total_tokens: 21260 } }, rate_limits: { limit_id: 'codex', primary: { used_percent: 38, window_minutes: 300 }, plan_type: 'pro' } } },
    'sample-tool': { timestamp: '2026-07-21T15:02:09Z', type: 'response_item', payload: { type: 'function_call', name: 'exec_command', call_id: 'sample-call', arguments: '{"cmd":"pnpm test","yield_time_ms":30000}' } },
    'sample-tool-result': { timestamp: '2026-07-21T15:02:10Z', type: 'response_item', payload: { type: 'function_call_output', call_id: 'sample-call', output: '42 tests passed' } },
    'sample-agent': { timestamp: '2026-07-21T15:11:02Z', type: 'response_item', payload: { type: 'message', role: 'assistant', content: [{ type: 'output_text', text: 'I found the dashboard, context map, and review report surfaces that need sample-backed interactions.' }] } },
    'sample-compacted': { timestamp: '2026-07-21T16:01:01Z', type: 'compacted', payload: { message: 'Recorded compaction checkpoint for the sample session.', window_number: 2, window_id: 'sample-window-2' } },
    'sample-response': { timestamp: '2026-07-21T17:38:02Z', type: 'response_item', payload: { type: 'message', role: 'assistant', content: [{ type: 'output_text', text: 'The interactive sample is ready with source-backed navigation and privacy-safe data.' }] } },
  }
  const text = JSON.stringify(records[evidenceId] ?? records['sample-user'])
  return { schemaVersion: 2, datasetEpoch: epoch, appliedRevision: revision, coverage: { fidelity: 'exact', observed: 1, eligible: 1 }, evidenceId, availability: 'available', availabilityObservedAt: now, availabilityRevision: revision, locator: { sourceId: 'sample-rollout', recordOrdinal: 3, byteStart: 10, byteEnd: 10 + text.length }, sourcePrefixSha256: 'a'.repeat(64), eventFingerprint: 'b'.repeat(64), offset: 0, bytes: text.length, text, encoding: 'utf-8', complete: true }
}

const reviewSpec = { reviewId: 'sample-review', datasetEpoch: epoch, indexRevision: revision, scope: { kind: 'single_session', rootSessionId: 'sample-build-week' }, model: 'gpt-5.6-sol', reasoning: 'high' }
const reviewSummary = { reviewId: 'sample-review', status: 'complete', createdAt: '2026-07-21T17:40:00Z', threadId: 'sample-thread' }
const reviewDetail = { summary: reviewSummary, review: reviewSpec, run: { schemaVersion: 'inspector.review/v1', reviewId: 'sample-review', status: 'complete', createdAt: '2026-07-21T17:40:00Z', startedAt: '2026-07-21T17:40:01Z', completedAt: '2026-07-21T17:41:20Z', threadId: null, launchPromptVersion: 'inspector.review-launch/v2', command: ['codex','exec'], acceptedReportSha256: 'c'.repeat(64), diagnostics: [], review: reviewSpec }, reportState: 'accepted', acceptedReport: { schemaVersion: 'inspector.review/v1', reviewId: 'sample-review', scope: { kind: 'single_session', summary: 'One root session and three delegated branches.', datasetEpoch: epoch, indexRevision: revision }, model: 'gpt-5.6-sol', reasoning: 'high', completedAt: '2026-07-21T17:41:20Z', summary: 'Strong source-backed observability with one opportunity to sharpen the first-run story.', findings: [
  { findingId: 'sample-strength', kind: 'strength', lens: 'execution_efficiency', title: 'Delegation stays causally visible', observation: 'The root session and spawned-agent work remain separate throughout the token map.', impact: 'Users can see where effort went without double-counting downstream work.', support: 'directly_observed', evidenceSummary: 'The cited token event and session topology show root and descendant usage separately.', citations: [{ evidenceId: 'sample-token', sourcePrefixSha256: 'a'.repeat(64), eventFingerprint: 'b'.repeat(64), availability: 'available', availabilityObservedAt: now, availabilityRevision: revision, rootSessionId: 'sample-build-week', turnId: 'sample-build-week-turn-1' }], recommendation: 'Keep the causal map as the primary proof point in the demo.', actionPrompt: 'Draft a 60-second demo narration centered on the causal token map.' },
  { findingId: 'sample-opportunity', kind: 'opportunity', lens: 'task_framing_and_steering', title: 'Make the first useful click explicit', observation: 'The dashboard contains several strong entry points, but a new viewer may not know which session to inspect first.', impact: 'The core evidence experience can take longer to discover during a short judging session.', support: 'inferred', evidenceSummary: 'The final response confirms the demo is complete, while the journey still depends on choosing a session.', citations: [{ evidenceId: 'sample-response', sourcePrefixSha256: 'a'.repeat(64), eventFingerprint: 'b'.repeat(64), availability: 'available', availabilityObservedAt: now, availabilityRevision: revision, rootSessionId: 'sample-build-week', turnId: 'sample-build-week-turn-1' }], recommendation: 'Use the most token-intensive session as the default guided path.', actionPrompt: 'Add a concise callout that points first-time viewers to the Build Week demo session.' },
] , contentSha256: 'c'.repeat(64) } }

function json(body: unknown, statusCode = 200) {
  return new Response(JSON.stringify(body), { status: statusCode, headers: { 'content-type': 'application/json' } })
}

export function installDemoApi() {
  const networkFetch = globalThis.fetch.bind(globalThis)
  globalThis.fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
    const raw = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
    const url = new URL(raw, location.origin)
    if (!url.pathname.startsWith('/v1/')) return networkFetch(input, init)
    if (url.pathname === '/v1/status') return json(status)
    if (url.pathname === '/v1/events') return new Response(null, { status: 204 })
    if (url.pathname === '/v1/metrics/catalog') return json({ schemaVersion: 2, datasetEpoch: epoch, appliedRevision: revision, coverage: exact, metrics: [{ key: 'recorded_tokens', formulaVersion: 1, unit: 'tokens', dimensions: [] }], filterOptions })
    if (url.pathname === '/v1/metrics/query') return json(metrics(init?.body ? JSON.parse(String(init.body)) : {}))
    if (url.pathname === '/v1/source-diagnostics') return json({ schemaVersion: 2, datasetEpoch: epoch, appliedRevision: revision, items: [] })
    if (url.pathname === '/v1/sessions') return json(sessionPage(url.searchParams.get('query') ?? ''))
    const mapMatch = url.pathname.match(/^\/v1\/sessions\/([^/]+)\/map$/)
    if (mapMatch) return json(sessionMap(decodeURIComponent(mapMatch[1])))
    const ledgerMatch = url.pathname.match(/^\/v1\/sessions\/([^/]+)\/turns\/([^/]+)\/ledger$/)
    if (ledgerMatch) return json(ledger(decodeURIComponent(ledgerMatch[2])))
    const evidenceMatch = url.pathname.match(/^\/v1\/evidence\/([^/]+)$/)
    if (evidenceMatch) return json(evidence(decodeURIComponent(evidenceMatch[1])))
    if (url.pathname === '/v1/review-plans' && init?.method === 'POST') return json({ schemaVersion: 3, datasetEpoch: epoch, appliedRevision: revision, planId: 'sample-plan', review: reviewSpec, launchPrompt: 'Use $codex-inspector:review-session. Review the selected sample scope across task framing, execution efficiency, delegation, and reusable leverage.' })
    if (url.pathname === '/v1/reviews' && init?.method === 'POST') return json({ reviewId: 'sample-review', status: 'planned', createdAt: now }, 202)
    if (url.pathname === '/v1/reviews') return json({ items: [reviewSummary] })
    if (url.pathname === '/v1/reviews/sample-review') return json(reviewDetail)
    if (url.pathname === '/v1/sync') return json({ accepted: true }, 202)
    return json({ title: 'Unavailable in sample', detail: 'This local-only action is disabled in the hosted sample.' }, 404)
  }
}
