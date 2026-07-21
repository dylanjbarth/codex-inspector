/* eslint-disable @typescript-eslint/no-explicit-any */

export const DEMO_MODE = import.meta.env.VITE_DEMO_MODE === 'true'

const epoch = 'sample-2026-07-22'
const revision = 12
const exact = { fidelity: 'exact', observed: 12, eligible: 12 }
const now = '2026-07-22T01:25:00Z'

type ContributionKind = 'user_root_direct' | 'descendant' | 'inspector_review' | 'other_orphan'
type TokenBreakdown = { userRootDirect: number; descendant: number; inspectorReview: number; otherOrphan: number }
type DemoSession = {
  sessionId: string
  rootWorkUnitId: string
  purpose: 'user'
  title: string
  project: string
  projectId: string
  model: string
  reasoning: string
  startedAt: string
  latestCompleted: string
  completedTurns: number
  descendantSessions: number
  matchCategories: string[]
  matchSnippets: { category: string; text: string }[]
  directTokens: number
  descendantTokens: number
  reviewTokens: number
  otherTokens: number
}

const sessions: DemoSession[] = [
  {
    sessionId: 'sample-build-week', rootWorkUnitId: 'sample-build-week', purpose: 'user',
    title: 'Prepare the Build Week demo', project: '/Users/demo/codex-inspector',
    projectId: 'codex-inspector', model: 'gpt-5.6-sol', reasoning: 'xhigh',
    startedAt: '2026-07-21T15:02:00Z', latestCompleted: '2026-07-21T17:38:00Z', completedTurns: 14,
    descendantSessions: 3, matchCategories: ['root: user message'], matchSnippets: [{ category: 'root: user message', text: 'Build an interactive demo that makes agent delegation and evidence easy to understand.' }],
    directTokens: 428000, descendantTokens: 193000, reviewTokens: 18000, otherTokens: 2000,
  },
  {
    sessionId: 'sample-search-fix', rootWorkUnitId: 'sample-search-fix', purpose: 'user',
    title: 'Fix session discovery for older IDs', project: '/Users/demo/codex-inspector',
    projectId: 'codex-inspector', model: 'gpt-5.6-sol', reasoning: 'high',
    startedAt: '2026-07-21T11:20:00Z', latestCompleted: '2026-07-21T12:08:00Z', completedTurns: 7,
    descendantSessions: 1, matchCategories: ['root: user message'], matchSnippets: [{ category: 'root: user message', text: 'Find why older completed session IDs return no matches and add a regression test.' }],
    directTokens: 214000, descendantTokens: 76000, reviewTokens: 8000, otherTokens: 1000,
  },
  {
    sessionId: 'sample-judge-skills', rootWorkUnitId: 'sample-judge-skills', purpose: 'user',
    title: 'Evaluate the project against Build Week criteria', project: '/Users/demo/codex-inspector',
    projectId: 'codex-inspector', model: 'gpt-5.6-sol', reasoning: 'high',
    startedAt: '2026-07-20T18:10:00Z', latestCompleted: '2026-07-20T19:31:00Z', completedTurns: 9,
    descendantSessions: 2, matchCategories: ['root: user message'], matchSnippets: [{ category: 'root: user message', text: 'Run the AI and human judge simulations and compare readiness with the previous baseline.' }],
    directTokens: 305000, descendantTokens: 119000, reviewTokens: 24000, otherTokens: 3000,
  },
  {
    sessionId: 'sample-dashboard-qa', rootWorkUnitId: 'sample-dashboard-qa', purpose: 'user',
    title: 'QA the token dashboard before the demo', project: '/Users/demo/codex-inspector',
    projectId: 'codex-inspector', model: 'gpt-5.6-terra', reasoning: 'high',
    startedAt: '2026-07-18T09:10:00Z', latestCompleted: '2026-07-18T10:22:00Z', completedTurns: 8,
    descendantSessions: 1, matchCategories: ['root: user message'], matchSnippets: [{ category: 'root: user message', text: 'Test every dashboard filter, record the exact failures, and verify the repaired flow.' }],
    directTokens: 168000, descendantTokens: 54000, reviewTokens: 11000, otherTokens: 1000,
  },
  {
    sessionId: 'sample-video-ablation', rootWorkUnitId: 'sample-video-ablation', purpose: 'user',
    title: 'Compare temporal pooling ablations', project: '/Users/demo/video-research',
    projectId: 'video-research', model: 'gpt-5.6-sol', reasoning: 'xhigh',
    startedAt: '2026-07-14T04:10:00Z', latestCompleted: '2026-07-14T06:02:00Z', completedTurns: 12,
    descendantSessions: 1, matchCategories: ['root: user message'], matchSnippets: [{ category: 'root: user message', text: 'Compare the 8-frame and 32-frame ablations, then explain the accuracy and latency tradeoff.' }],
    directTokens: 412000, descendantTokens: 61000, reviewTokens: 14000, otherTokens: 2000,
  },
  {
    sessionId: 'sample-video-figure', rootWorkUnitId: 'sample-video-figure', purpose: 'user',
    title: 'Turn benchmark results into a paper figure', project: '/Users/demo/video-research',
    projectId: 'video-research', model: 'gpt-5.6-terra', reasoning: 'high',
    startedAt: '2026-07-09T07:40:00Z', latestCompleted: '2026-07-09T08:28:00Z', completedTurns: 6,
    descendantSessions: 0, matchCategories: ['root: user message'], matchSnippets: [{ category: 'root: user message', text: 'Create a publication-ready comparison figure from the verified benchmark table.' }],
    directTokens: 277000, descendantTokens: 28000, reviewTokens: 10000, otherTokens: 1000,
  },
  {
    sessionId: 'sample-related-work', rootWorkUnitId: 'sample-related-work', purpose: 'user',
    title: 'Map the related work for video token selection', project: '/Users/demo/video-research',
    projectId: 'video-research', model: 'gpt-5.6-sol', reasoning: 'xhigh',
    startedAt: '2026-06-30T13:05:00Z', latestCompleted: '2026-06-30T15:19:00Z', completedTurns: 11,
    descendantSessions: 2, matchCategories: ['root: user message'], matchSnippets: [{ category: 'root: user message', text: 'Build an evidence-backed map of recent video token selection methods and open questions.' }],
    directTokens: 356000, descendantTokens: 92000, reviewTokens: 19000, otherTokens: 2000,
  },
  {
    sessionId: 'sample-event-page', rootWorkUnitId: 'sample-event-page', purpose: 'user',
    title: 'Launch the July community meetup page', project: '/Users/demo/community-ops',
    projectId: 'community-ops', model: 'gpt-5.6-terra', reasoning: 'medium',
    startedAt: '2026-06-22T07:30:00Z', latestCompleted: '2026-06-22T08:16:00Z', completedTurns: 5,
    descendantSessions: 1, matchCategories: ['root: user message'], matchSnippets: [{ category: 'root: user message', text: 'Publish the meetup page with the confirmed schedule, speakers, and registration details.' }],
    directTokens: 191000, descendantTokens: 36000, reviewTokens: 7000, otherTokens: 4000,
  },
  {
    sessionId: 'sample-speaker-outreach', rootWorkUnitId: 'sample-speaker-outreach', purpose: 'user',
    title: 'Prepare speaker outreach and scheduling', project: '/Users/demo/community-ops',
    projectId: 'community-ops', model: 'gpt-5.6-terra', reasoning: 'medium',
    startedAt: '2026-06-12T03:20:00Z', latestCompleted: '2026-06-12T04:04:00Z', completedTurns: 5,
    descendantSessions: 0, matchCategories: ['root: user message'], matchSnippets: [{ category: 'root: user message', text: 'Draft concise speaker outreach and identify scheduling conflicts before anything is sent.' }],
    directTokens: 144000, descendantTokens: 18000, reviewTokens: 5000, otherTokens: 3000,
  },
  {
    sessionId: 'sample-community-recap', rootWorkUnitId: 'sample-community-recap', purpose: 'user',
    title: 'Write the community meetup recap', project: '/Users/demo/community-ops',
    projectId: 'community-ops', model: 'gpt-5.6-terra', reasoning: 'high',
    startedAt: '2026-06-02T10:00:00Z', latestCompleted: '2026-06-02T11:03:00Z', completedTurns: 7,
    descendantSessions: 0, matchCategories: ['root: user message'], matchSnippets: [{ category: 'root: user message', text: 'Turn the event notes into a factual recap without inventing quotes or attendance claims.' }],
    directTokens: 205000, descendantTokens: 0, reviewTokens: 12000, otherTokens: 8000,
  },
  {
    sessionId: 'sample-inspector-architecture', rootWorkUnitId: 'sample-inspector-architecture', purpose: 'user',
    title: 'Define the phased Inspector architecture', project: '/Users/demo/codex-inspector',
    projectId: 'codex-inspector', model: 'gpt-5.6-sol', reasoning: 'xhigh',
    startedAt: '2026-05-26T05:12:00Z', latestCompleted: '2026-05-26T07:46:00Z', completedTurns: 13,
    descendantSessions: 3, matchCategories: ['root: user message'], matchSnippets: [{ category: 'root: user message', text: 'Turn the MVP architecture into implementation phases with explicit contracts and risks.' }],
    directTokens: 382000, descendantTokens: 158000, reviewTokens: 31000, otherTokens: 6000,
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
  projects: [
    { value: 'codex-inspector', label: 'Codex Inspector' },
    { value: 'video-research', label: 'Video Research' },
    { value: 'community-ops', label: 'Community Ops' },
  ],
  models: ['gpt-5.6-sol', 'gpt-5.6-terra'], reasoningEfforts: ['medium', 'high', 'xhigh'],
  contributionKinds: [
    { value: 'user_root_direct', label: 'Direct user-session work' },
    { value: 'descendant', label: 'Spawned agent work' },
    { value: 'inspector_review', label: 'Codex Inspector reviews' },
    { value: 'other_orphan', label: 'Other or unlinked session work' },
  ],
}

const dayMs = 86400000
const demoReferenceEnd = Date.parse(now)
const capacityUse = [6, 13, 21, 30, 41, 53, 64]
const capacityObservations = Array.from({ length: 60 }, (_, index) => {
  const observedAt = demoReferenceEnd - (59 - index) * dayMs
  const cycleDay = index % 7
  const resetBoundary = observedAt + (7 - cycleDay) * dayMs
  return { observedAt: new Date(observedAt).toISOString(), usedPercent: capacityUse[cycleDay], resetBoundary: new Date(resetBoundary).toISOString() }
})

function selectedKinds(request: any): Set<ContributionKind> {
  const requested = Array.isArray(request.contributionKinds) ? request.contributionKinds : []
  return new Set<ContributionKind>(requested.length ? requested : ['user_root_direct', 'descendant', 'inspector_review', 'other_orphan'])
}

function sessionBreakdown(session: DemoSession, kinds: Set<ContributionKind>): TokenBreakdown {
  return {
    userRootDirect: kinds.has('user_root_direct') ? session.directTokens : 0,
    descendant: kinds.has('descendant') ? session.descendantTokens : 0,
    inspectorReview: kinds.has('inspector_review') ? session.reviewTokens : 0,
    otherOrphan: kinds.has('other_orphan') ? session.otherTokens : 0,
  }
}

function sumBreakdowns(values: TokenBreakdown[]): TokenBreakdown {
  return values.reduce((sum, value) => ({
    userRootDirect: sum.userRootDirect + value.userRootDirect,
    descendant: sum.descendant + value.descendant,
    inspectorReview: sum.inspectorReview + value.inspectorReview,
    otherOrphan: sum.otherOrphan + value.otherOrphan,
  }), { userRootDirect: 0, descendant: 0, inspectorReview: 0, otherOrphan: 0 })
}

function demoRange(request: any) {
  const requestedStart = Date.parse(request.start)
  const requestedEnd = Date.parse(request.end)
  const duration = Number.isFinite(requestedStart) && Number.isFinite(requestedEnd)
    ? Math.max(dayMs, Math.min(60 * dayMs, requestedEnd - requestedStart))
    : 30 * dayMs
  return { start: demoReferenceEnd - duration, end: demoReferenceEnd, requestedStart: request.start ?? null, requestedEnd: request.end ?? null }
}

function metricMeta(request: any, selectedCount: number) {
  const range = demoRange(request)
  const coverage = { fidelity: 'exact', observed: selectedCount, eligible: selectedCount }
  return {
    formulaVersion: 1, fidelity: 'exact', coverage,
    indexedCoverage: { indexedStart: '2026-05-24T01:25:00Z', indexedEnd: now, completedWatermark: now },
    timeBoundary: { requestedStart: range.requestedStart, requestedEnd: range.requestedEnd, effectiveStart: new Date(range.start).toISOString(), effectiveEnd: now, timezone: request.timezone ?? 'UTC' },
    exclusionReasons: {},
  }
}

export function buildDemoMetrics(request: any = {}) {
  const range = demoRange(request)
  const kinds = selectedKinds(request)
  const projectIds = new Set<string>(request.projectIds ?? [])
  const models = new Set<string>(request.models ?? [])
  const reasoningEfforts = new Set<string>(request.reasoningEfforts ?? [])
  const selected = sessions.filter(session => {
    const startedAt = Date.parse(session.startedAt)
    return startedAt >= range.start && startedAt <= range.end
      && (!projectIds.size || projectIds.has(session.projectId))
      && (!models.size || models.has(session.model))
      && (!reasoningEfforts.size || reasoningEfforts.has(session.reasoning))
  })
  const breakdowns = selected.map(session => sessionBreakdown(session, kinds))
  const byKind = sumBreakdowns(breakdowns)
  const total = Object.values(byKind).reduce((sum, value) => sum + value, 0)
  const grain = request.grain === 'hour' ? 'hour' : 'day'
  const bucketMs = grain === 'hour' ? 3600000 : dayMs
  const buckets = new Map<number, TokenBreakdown>()
  const cachedBuckets = new Map<number, TokenBreakdown>()
  const modelReasoningBuckets = new Map<number, Map<string, { model: string; reasoningEffort: string; tokens: number; uncachedTokens: number; cachedTokens: number }>>()
  selected.forEach((session, index) => {
    const startedAt = Date.parse(session.startedAt)
    const bucketStart = grain === 'hour'
      ? Math.floor(startedAt / bucketMs) * bucketMs
      : Date.UTC(new Date(startedAt).getUTCFullYear(), new Date(startedAt).getUTCMonth(), new Date(startedAt).getUTCDate())
    const breakdown = breakdowns[index]
    const cachedShare = session.model === 'gpt-5.6-sol' ? .34 : .24
    const cached = {
      userRootDirect: Math.round(breakdown.userRootDirect * cachedShare),
      descendant: Math.round(breakdown.descendant * cachedShare),
      inspectorReview: Math.round(breakdown.inspectorReview * cachedShare),
      otherOrphan: Math.round(breakdown.otherOrphan * cachedShare),
    }
    buckets.set(bucketStart, sumBreakdowns([buckets.get(bucketStart) ?? { userRootDirect: 0, descendant: 0, inspectorReview: 0, otherOrphan: 0 }, breakdown]))
    cachedBuckets.set(bucketStart, sumBreakdowns([cachedBuckets.get(bucketStart) ?? { userRootDirect: 0, descendant: 0, inspectorReview: 0, otherOrphan: 0 }, cached]))
    const series = modelReasoningBuckets.get(bucketStart) ?? new Map()
    const seriesKey = `${session.model}\u0000${session.reasoning}`
    const tokens = Object.values(breakdown).reduce((sum, value) => sum + value, 0)
    const cachedTokens = Object.values(cached).reduce((sum, value) => sum + value, 0)
    const current = series.get(seriesKey) ?? { model: session.model, reasoningEffort: session.reasoning, tokens: 0, uncachedTokens: 0, cachedTokens: 0 }
    current.tokens += tokens
    current.cachedTokens += cachedTokens
    current.uncachedTokens += tokens - cachedTokens
    series.set(seriesKey, current)
    modelReasoningBuckets.set(bucketStart, series)
  })
  const overTime = [...buckets.entries()].sort(([a], [b]) => a - b).map(([bucketStart, byKind]) => ({
    bucketStart: new Date(bucketStart).toISOString(),
    bucketEnd: new Date(bucketStart + bucketMs).toISOString(),
    timezone: request.timezone ?? 'UTC',
    grain,
    byKind,
    byKindCached: cachedBuckets.get(bucketStart) ?? { userRootDirect: 0, descendant: 0, inspectorReview: 0, otherOrphan: 0 },
    byKindUncached: sumBreakdowns([
      byKind,
      Object.fromEntries(Object.entries(cachedBuckets.get(bucketStart) ?? {}).map(([key, value]) => [key, -value])) as TokenBreakdown,
    ]),
  }))
  const modelReasoningOverTime = [...modelReasoningBuckets.entries()].sort(([a], [b]) => a - b).map(([bucketStart, series]) => ({
    bucketStart: new Date(bucketStart).toISOString(),
    bucketEnd: new Date(bucketStart + bucketMs).toISOString(),
    timezone: request.timezone ?? 'UTC',
    grain,
    series: [...series.values()].sort((a, b) => b.tokens - a.tokens),
  }))
  const composition = selected.reduce((sum, session, index) => {
    const amount = Object.values(breakdowns[index]).reduce((total, value) => total + value, 0)
    const cachedShare = session.model === 'gpt-5.6-sol' ? .34 : .24
    const visibleShare = session.model === 'gpt-5.6-sol' ? .14 : .17
    const reasoningShare = session.reasoning === 'xhigh' ? .14 : session.reasoning === 'high' ? .08 : .04
    const uncachedShare = 1 - cachedShare - visibleShare - reasoningShare
    sum.uncachedInput += Math.round(amount * uncachedShare)
    sum.cachedInput += Math.round(amount * cachedShare)
    sum.visibleOutput += Math.round(amount * visibleShare)
    sum.reasoningOutput += Math.round(amount * reasoningShare)
    return sum
  }, { uncachedInput: 0, cachedInput: 0, visibleOutput: 0, reasoningOutput: 0, residual: 0 })
  composition.residual = total - composition.uncachedInput - composition.cachedInput - composition.visibleOutput - composition.reasoningOutput
  const roots = selected.map(session => {
    const directTokens = kinds.has('user_root_direct') ? session.directTokens : 0
    const descendantTokens = kinds.has('descendant') ? session.descendantTokens : 0
    return { rootSessionId: session.sessionId, inclusiveTokens: directTokens + descendantTokens, directTokens, descendantTokens }
  }).filter(root => root.inclusiveTokens > 0).sort((a, b) => b.inclusiveTokens - a.inclusiveTokens)
  const capacityGroups = new Map<string, { limitId: string; windowMinutes: number; resetBoundary: string; points: { observedAt: string; usedPercent: number }[] }>()
  capacityObservations.filter(point => Date.parse(point.observedAt) >= range.start).forEach(point => {
    const group = capacityGroups.get(point.resetBoundary) ?? { limitId: 'codex', windowMinutes: 10080, resetBoundary: point.resetBoundary, points: [] }
    group.points.push({ observedAt: point.observedAt, usedPercent: point.usedPercent })
    capacityGroups.set(point.resetBoundary, group)
  })
  const meta = metricMeta(request, selected.length)
  const coverage = meta.coverage
  return { schemaVersion: 2, datasetEpoch: epoch, appliedRevision: revision, coverage, results: [
    { ...meta, key: 'recorded_tokens', value: total },
    { ...meta, key: 'recorded_tokens_by_kind', value: byKind },
    { ...meta, key: 'recorded_tokens_over_time', value: overTime },
    { ...meta, key: 'recorded_tokens_by_model_reasoning_over_time', value: modelReasoningOverTime },
    { ...meta, key: 'token_composition', value: composition },
    { ...meta, key: 'top_root_sessions_by_tokens', value: roots },
    { ...meta, key: 'latest_capacity_observation', value: [{ observedAt: now, limitId: 'codex', windowMinutes: 300, usedPercent: 38, remainingPercent: 62, resetsAt: '2026-07-22T05:00:00Z', stale: false }, { observedAt: now, limitId: 'codex', windowMinutes: 10080, usedPercent: 64, remainingPercent: 36, resetsAt: '2026-07-27T00:00:00Z', stale: false }] },
    { ...meta, key: 'capacity_drawdown', value: [...capacityGroups.values()] },
  ] }
}

function sessionPage(query = '', rootIds: string[] = []) {
  const needle = query.trim().toLowerCase()
  const roots = new Set(rootIds)
  const items = sessions.filter(item => (!roots.size || roots.has(item.sessionId)) && (!needle || [item.title, item.project, ...item.matchSnippets.map(snippet => snippet.text)].join(' ').toLowerCase().includes(needle)))
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
    if (url.pathname === '/v1/metrics/query') return json(buildDemoMetrics(init?.body ? JSON.parse(String(init.body)) : {}))
    if (url.pathname === '/v1/source-diagnostics') return json({ schemaVersion: 2, datasetEpoch: epoch, appliedRevision: revision, items: [] })
    if (url.pathname === '/v1/sessions') return json(sessionPage(url.searchParams.get('query') ?? '', url.searchParams.getAll('rootId')))
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
