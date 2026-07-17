import assert from "node:assert/strict";
import test from "node:test";

import { JSDOM } from "jsdom";

import { renderStandaloneReport } from "../src/report-html.mjs";

test("connects cross-session comparison to the selected session story", async (t) => {
  const html = await renderStandaloneReport(reportFixture());
  const dom = new JSDOM(html, {
    runScripts: "dangerously",
    url: "https://codex-inspector.local/",
  });
  t.after(() => dom.window.close());

  const document = dom.window.document;
  assert.equal(document.querySelector("h1")?.textContent, "Codex Inspector");
  assert.match(document.body.textContent, /Sessions worth a look/);
  assert.match(document.body.textContent, /Session story/);
  assert.match(document.body.textContent, /Longest measured tool calls/);
  assert.match(document.body.textContent, /Agent orchestration/);

  const sessionButtons = [...document.querySelectorAll('button[data-session-id]')];
  assert.equal(sessionButtons.length, 2);
  assert.equal(sessionButtons[0].getAttribute("aria-pressed"), "true");
  assert.match(document.querySelector('[data-testid="session-story"]')?.textContent, /session-a1b2c3d4/);
  assert.match(document.querySelector('[data-testid="session-story"]')?.textContent, /12K/);
  assert.match(document.querySelector('[data-testid="session-story"]')?.textContent, /Turn 1/);

  sessionButtons[1].dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));

  const selectedButtons = [...document.querySelectorAll('button[data-session-id]')];
  assert.equal(selectedButtons[1].getAttribute("aria-pressed"), "true");
  const story = document.querySelector('[data-testid="session-story"]')?.textContent;
  assert.match(story, /session-e5f6g7h8/);
  assert.match(story, /4K/);
  assert.match(story, /slow_connector/);
  assert.doesNotMatch(story, /fetch_metrics/);
});

test("keeps turn details collapsed until the engineer expands a turn", async (t) => {
  const html = await renderStandaloneReport(reportFixture());
  const dom = new JSDOM(html, {
    runScripts: "dangerously",
    url: "https://codex-inspector.local/",
  });
  t.after(() => dom.window.close());

  const document = dom.window.document;
  let toggles = [...document.querySelectorAll("button[data-turn-toggle]")];
  assert.equal(toggles.length, 2);
  assert.deepEqual(toggles.map((toggle) => toggle.getAttribute("aria-expanded")), ["false", "false"]);

  const detailsId = toggles[0].getAttribute("aria-controls");
  assert.equal(document.getElementById(detailsId)?.hidden, true);

  toggles[0].dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));

  toggles = [...document.querySelectorAll("button[data-turn-toggle]")];
  assert.equal(toggles[0].getAttribute("aria-expanded"), "true");
  assert.equal(document.getElementById(detailsId)?.hidden, false);
  assert.match(document.getElementById(detailsId)?.textContent, /fetch_metrics/);

  toggles[0].dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));

  toggles = [...document.querySelectorAll("button[data-turn-toggle]")];
  assert.equal(toggles[0].getAttribute("aria-expanded"), "false");
  assert.equal(document.getElementById(detailsId)?.hidden, true);
});

function reportFixture() {
  const first = session({
    displayId: "session-a1b2c3d4",
    totalTokens: 12_000,
    turnCount: 2,
    totalDurationMs: 180_000,
    compactions: 2,
    toolName: "fetch_metrics",
    toolStatus: "success",
    toolDurationMs: 2_500,
    spawnedSubagents: 2,
    subagentTokens: 3_000,
  });
  const second = session({
    displayId: "session-e5f6g7h8",
    totalTokens: 4_000,
    turnCount: 1,
    totalDurationMs: 45_000,
    compactions: 0,
    toolName: "slow_connector",
    toolStatus: "incomplete",
    toolDurationMs: null,
    spawnedSubagents: 0,
    subagentTokens: 0,
  });

  return {
    schemaVersion: 1,
    surface: "dashboard",
    generatedAt: "2026-07-17T12:00:00.000Z",
    window: { days: 30, label: "Last 30 days" },
    metrics: { sessions: 2, turns: 3, tokenUsage: 16_000, toolCalls: 2, compactions: 2 },
    finding: {
      id: "compaction_churn.v1",
      title: "Repeated compaction in a continuing session",
      triggered: true,
      affectedSessionCount: 1,
      summary: "One session continued after repeated compaction.",
      interpretation: "This is a review signal, not a verdict.",
      recommendation: "Consider a clean handoff before the next turn.",
      sessions: [],
    },
    analysis: {
      comparison: {
        tokenUsage: {
          eligibleSessions: 2,
          median: 8_000,
          p90: 12_000,
          topSessions: [
            { displayId: first.displayId, value: 12_000 },
            { displayId: second.displayId, value: 4_000 },
          ],
        },
        turns: { median: 1.5, p90: 2, topSessions: [] },
        compactions: { sessionsWithCompactions: 1, repeatedSessions: 1, rate: 0.5, topSessions: [] },
        turnDuration: { eligibleTurns: 3, median: 60_000, p90: 120_000, topSessions: [] },
        tools: { calls: 2, completedCalls: 1, incompleteCalls: 1, topSessions: [] },
        expensiveToolCalls: {
          basis: "recorded_duration",
          eligibleCalls: 1,
          topCalls: [
            {
              sessionDisplayId: first.displayId,
              turnLabel: "Turn 1",
              name: "fetch_metrics",
              durationMs: 2_500,
              status: "success",
            },
          ],
        },
        orchestration: {
          workflows: 2,
          workflowsWithSubagents: 1,
          spawnedSubagents: 2,
          maxDepth: 1,
          rootTokens: 13_000,
          subagentTokens: 3_000,
          subagentTokenShare: 3 / 16,
          topWorkflows: [],
        },
      },
      sessions: [first, second],
    },
    coverage: { compatibleSessions: 2, incompatibleSessions: 0, warnings: {} },
  };
}

function session({
  displayId,
  totalTokens,
  turnCount,
  totalDurationMs,
  compactions,
  toolName,
  toolStatus,
  toolDurationMs,
  spawnedSubagents,
  subagentTokens,
}) {
  const tools = [{ name: toolName, category: "mcp_or_app", status: toolStatus, durationMs: toolDurationMs }];
  return {
    displayId,
    startedAt: "2026-07-16T08:00:00.000Z",
    lastObservedAt: "2026-07-16T08:05:00.000Z",
    tokenUsage: {
      total: totalTokens,
      input: Math.round(totalTokens * 0.65),
      cachedInput: Math.round(totalTokens * 0.2),
      output: Math.round(totalTokens * 0.25),
      reasoningOutput: Math.round(totalTokens * 0.1),
    },
    turns: {
      count: turnCount,
      totalDurationMs,
      medianDurationMs: totalDurationMs / turnCount,
      p90DurationMs: totalDurationMs / turnCount,
      items: Array.from({ length: turnCount }, (_, index) => ({
        label: `Turn ${index + 1}`,
        startedAt: "2026-07-16T08:01:00.000Z",
        completedAt: "2026-07-16T08:03:00.000Z",
        status: index === 0 ? "completed" : "incomplete",
        durationMs: totalDurationMs / turnCount,
        timeToFirstTokenMs: 1_500,
        tokenUsage: { total: totalTokens / turnCount, input: null, cachedInput: null, output: null, reasoningOutput: null },
        compactions: index === 0 ? compactions : 0,
        tools: index === 0 ? tools : [],
      })),
    },
    compactions: { count: compactions },
    tools: {
      summary: {
        calls: 1,
        success: toolStatus === "success" ? 1 : 0,
        failed: 0,
        completed: 0,
        incomplete: toolStatus === "incomplete" ? 1 : 0,
        completionRate: toolStatus === "incomplete" ? 0 : 1,
      },
    },
    orchestration: {
      role: "root",
      depth: 0,
      parentDisplayId: null,
      rootDisplayId: displayId,
      directSubagents: spawnedSubagents,
      spawnedSubagents,
      maxDepth: spawnedSubagents ? 1 : 0,
      rootTokens: totalTokens,
      subagentTokens,
      totalWorkflowTokens: totalTokens + subagentTokens,
      children: spawnedSubagents
        ? [{ displayId: "session-child123", parentDisplayId: displayId, depth: 1, tokenUsage: subagentTokens, turns: 1, toolCalls: 1 }]
        : [],
    },
  };
}
