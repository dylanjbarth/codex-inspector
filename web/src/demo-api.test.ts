import { describe, expect, it } from 'vitest'
import { buildDemoMetrics } from './demo-api'

const end = Date.parse('2026-07-22T01:25:00Z')

function query(days: number, filters: Record<string, string[]> = {}) {
  return buildDemoMetrics({
    metricKeys: [],
    timezone: 'UTC',
    grain: days <= 2 ? 'hour' : 'day',
    start: new Date(end - days * 86400000).toISOString(),
    end: new Date(end).toISOString(),
    ...filters,
  })
}

function value(result: ReturnType<typeof query>, key: string): unknown {
  return result.results.find(metric => metric.key === key)?.value
}

function total(result: ReturnType<typeof query>): number {
  return Number(value(result, 'recorded_tokens'))
}

function roots(result: ReturnType<typeof query>) {
  return value(result, 'top_root_sessions_by_tokens') as { rootSessionId: string; inclusiveTokens: number; directTokens: number; descendantTokens: number }[]
}

describe('hosted demo dataset', () => {
  it('expands through distinct, internally consistent work history as the time range grows', () => {
    const hours48 = query(2)
    const days7 = query(7)
    const days30 = query(30)
    const days60 = query(60)

    expect([roots(hours48).length, roots(days7).length, roots(days30).length, roots(days60).length]).toEqual([3, 4, 8, 11])
    expect(total(hours48)).toBeLessThan(total(days7))
    expect(total(days7)).toBeLessThan(total(days30))
    expect(total(days30)).toBeLessThan(total(days60))
  })

  it('uses real session attributes for project, model, and reasoning intersections', () => {
    const videoResearch = query(30, { projectIds: ['video-research'] })
    const communityOps = query(30, { projectIds: ['community-ops'] })
    const impossibleSlice = query(60, { projectIds: ['video-research'], reasoningEfforts: ['medium'] })

    expect(roots(videoResearch).map(root => root.rootSessionId)).toEqual(['sample-video-ablation', 'sample-related-work', 'sample-video-figure'])
    expect(roots(communityOps).map(root => root.rootSessionId)).toEqual(['sample-event-page'])
    expect(total(videoResearch)).not.toBe(total(communityOps))
    expect(total(impossibleSlice)).toBe(0)
  })

  it('recalculates totals, composition inputs, and root attribution for contribution filters', () => {
    const all = query(30)
    const descendants = query(30, { contributionKinds: ['descendant'] })
    const byKind = value(descendants, 'recorded_tokens_by_kind') as { userRootDirect: number; descendant: number; inspectorReview: number; otherOrphan: number }

    expect(total(descendants)).toBe(byKind.descendant)
    expect(total(descendants)).toBeLessThan(total(all))
    expect(byKind.userRootDirect).toBe(0)
    expect(byKind.inspectorReview).toBe(0)
    expect(roots(descendants).every(root => root.directTokens === 0 && root.descendantTokens > 0)).toBe(true)
  })
})
