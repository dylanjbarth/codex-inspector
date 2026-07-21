import assert from "node:assert/strict";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { analyzeSessions, InspectorError } from "../src/analyzer.mjs";

const NOW = new Date("2026-07-17T12:00:00.000Z");

test("aggregates five metrics and triggers compaction_churn.v1 across merged files", async (t) => {
  const root = await fixtureRoot(t);
  const privatePrompt = "PRIVATE_PROMPT_SENTINEL do not render";
  const privatePath = "/Users/example/secret-project";

  await jsonl(join(root, "session-a-1.jsonl"), [
    sessionMeta("session-a-full-id", "2026-07-16T08:00:00.000Z", privatePath),
    taskStarted("turn-1", "2026-07-16T08:01:00.000Z"),
    tokenCount(100, "2026-07-16T08:02:00.000Z"),
    compaction("2026-07-16T08:05:00.000Z"),
    userMessage(privatePrompt, "2026-07-16T08:05:30.000Z"),
    taskStarted("turn-2", "2026-07-16T08:06:00.000Z"),
  ]);
  await jsonl(join(root, "session-a-2.jsonl"), [
    sessionMeta("session-a-full-id", "2026-07-16T08:00:00.000Z", privatePath),
    functionCall("call-1", "SECRET_ARGUMENT", "2026-07-16T08:07:00.000Z"),
    functionCall("call-1", "SECRET_ARGUMENT", "2026-07-16T08:07:00.000Z"),
    customToolCall("call-2", "SECRET_INPUT", "2026-07-16T08:08:00.000Z"),
    tokenCount(250, "2026-07-16T08:09:00.000Z"),
    compaction("2026-07-16T08:10:00.000Z"),
  ]);
  await jsonl(join(root, "session-b.jsonl"), [
    sessionMeta("session-b-full-id", "2026-07-15T08:00:00.000Z", privatePath),
    taskStarted("turn-b", "2026-07-15T08:01:00.000Z"),
  ]);

  const report = await analyzeSessions({ sessionsRoot: root, sinceDays: 30, now: NOW });

  assert.deepEqual(
    {
      sessions: report.metrics.sessions,
      turns: report.metrics.turns,
      tokenUsage: report.metrics.tokenUsage,
      toolCalls: report.metrics.toolCalls,
      compactions: report.metrics.compactions,
    },
    { sessions: 2, turns: 3, tokenUsage: 250, toolCalls: 2, compactions: 2 },
  );
  assert.equal(report.finding.triggered, true);
  assert.equal(report.finding.affectedSessionCount, 1);
  assert.equal(report.finding.sessions[0].turnsAfterFirstCompaction, 1);
  assert.equal(report.finding.sessions[0].timeline.filter((event) => event.kind === "context_compacted").length, 2);

  const renderedData = JSON.stringify(report);
  for (const secret of [privatePrompt, privatePath, "SECRET_ARGUMENT", "SECRET_INPUT", "session-a-full-id"]) {
    assert.equal(renderedData.includes(secret), false, `report leaked ${secret}`);
  }
});

test("reports an honest no-finding state without changing the selected window", async (t) => {
  const root = await fixtureRoot(t);
  await jsonl(join(root, "healthy.jsonl"), [
    sessionMeta("healthy-session", "2026-07-16T08:00:00.000Z"),
    taskStarted("turn-1", "2026-07-16T08:01:00.000Z"),
    compaction("2026-07-16T08:05:00.000Z"),
    taskStarted("turn-2", "2026-07-16T08:06:00.000Z"),
  ]);

  const report = await analyzeSessions({ sessionsRoot: root, sinceDays: 7, now: NOW });
  assert.equal(report.window.days, 7);
  assert.equal(report.finding.triggered, false);
  assert.equal(report.finding.affectedSessionCount, 0);
  assert.match(report.finding.summary, /last 7 days/i);
});

test("fails closed when every recognizable in-window session is incompatible", async (t) => {
  const root = await fixtureRoot(t);
  await jsonl(join(root, "incompatible.jsonl"), [
    sessionMeta("incompatible-session", "2026-07-16T08:00:00.000Z"),
    { type: "event_msg", timestamp: "2026-07-16T08:01:00.000Z", payload: { type: "future_turn_started" } },
  ]);

  await assert.rejects(
    () => analyzeSessions({ sessionsRoot: root, sinceDays: 30, now: NOW }),
    (error) => error instanceof InspectorError && error.code === "unsupported_schema",
  );
});

test("skips malformed lines and reports the warning count", async (t) => {
  const root = await fixtureRoot(t);
  const lines = [
    JSON.stringify(sessionMeta("session-with-warning", "2026-07-16T08:00:00.000Z")),
    "{not valid json",
    JSON.stringify(taskStarted("turn-1", "2026-07-16T08:01:00.000Z")),
  ];
  await writeFile(join(root, "warning.jsonl"), `${lines.join("\n")}\n`, "utf8");

  const report = await analyzeSessions({ sessionsRoot: root, sinceDays: 30, now: NOW });
  assert.equal(report.metrics.sessions, 1);
  assert.equal(report.coverage.warnings.malformedLines, 1);
});

test("compares sessions while preserving token and duration evidence inside each turn", async (t) => {
  const root = await fixtureRoot(t);
  await jsonl(join(root, "session-a.jsonl"), [
    sessionMeta("session-a", "2026-07-16T08:00:00.000Z"),
    taskStarted("turn-a1", "2026-07-16T08:01:00.000Z"),
    tokenCount(100, "2026-07-16T08:02:00.000Z", {
      input: 70,
      cachedInput: 20,
      output: 20,
      reasoningOutput: 10,
    }),
    compaction("2026-07-16T08:02:30.000Z"),
    taskComplete("turn-a1", "2026-07-16T08:03:00.000Z", 10_000, 2_000),
    taskStarted("turn-a2", "2026-07-16T08:04:00.000Z"),
    tokenCount(250, "2026-07-16T08:05:00.000Z", {
      input: 100,
      cachedInput: 30,
      output: 35,
      reasoningOutput: 15,
      lastTotal: 150,
      totalInput: 170,
      totalCachedInput: 50,
      totalOutput: 55,
    }),
    compaction("2026-07-16T08:05:30.000Z"),
    taskComplete("turn-a2", "2026-07-16T08:06:00.000Z", 20_000, 3_000),
  ]);
  await jsonl(join(root, "session-b.jsonl"), [
    sessionMeta("session-b", "2026-07-15T08:00:00.000Z"),
    taskStarted("turn-b1", "2026-07-15T08:01:00.000Z"),
    tokenCount(50, "2026-07-15T08:02:00.000Z", {
      input: 30,
      cachedInput: 5,
      output: 15,
      reasoningOutput: 5,
    }),
    taskComplete("turn-b1", "2026-07-15T08:03:00.000Z", 5_000, 1_500),
  ]);

  const report = await analyzeSessions({ sessionsRoot: root, sinceDays: 30, now: NOW });

  assert.deepEqual(report.analysis.comparison.tokenUsage, {
    eligibleSessions: 2,
    median: 150,
    p90: 250,
    topSessions: [
      { displayId: report.analysis.sessions[0].displayId, value: 250 },
      { displayId: report.analysis.sessions[1].displayId, value: 50 },
    ],
  });
  assert.equal(report.analysis.comparison.turns.topSessions[0].value, 2);
  assert.equal(report.analysis.comparison.compactions.topSessions[0].value, 2);
  assert.equal(report.analysis.comparison.turnDuration.median, 10_000);
  assert.equal(report.analysis.comparison.turnDuration.p90, 20_000);

  const detailed = report.analysis.sessions[0];
  assert.deepEqual(detailed.tokenUsage, {
    total: 250,
    input: 170,
    cachedInput: 50,
    output: 55,
    reasoningOutput: 25,
  });
  assert.equal(detailed.turns.count, 2);
  assert.equal(detailed.turns.totalDurationMs, 30_000);
  assert.deepEqual(
    detailed.turns.items.map((turn) => ({
      label: turn.label,
      tokens: turn.tokenUsage.total,
      durationMs: turn.durationMs,
      compactions: turn.compactions,
    })),
    [
      { label: "Turn 1", tokens: 100, durationMs: 10_000, compactions: 1 },
      { label: "Turn 2", tokens: 150, durationMs: 20_000, compactions: 1 },
    ],
  );
});

test("explains tool completion inside a turn and ranks expensive calls by recorded duration", async (t) => {
  const root = await fixtureRoot(t);
  await jsonl(join(root, "tools.jsonl"), [
    sessionMeta("tool-session", "2026-07-16T08:00:00.000Z"),
    taskStarted("turn-tools", "2026-07-16T08:01:00.000Z"),
    functionCall("call-shell", "PRIVATE_COMMAND", "2026-07-16T08:02:00.000Z", {
      name: "exec_command",
      turnId: "turn-tools",
    }),
    functionCallOutput("call-shell", "PRIVATE_OUTPUT", "2026-07-16T08:02:30.000Z"),
    customToolCall("call-mcp", "PRIVATE_INPUT", "2026-07-16T08:03:00.000Z", {
      name: "fetch_metrics",
      turnId: "turn-tools",
    }),
    mcpToolEnd("call-mcp", "2026-07-16T08:03:03.000Z", 2_500, false),
    customToolCall("call-incomplete", "PRIVATE_INPUT", "2026-07-16T08:04:00.000Z", {
      name: "slow_connector",
      turnId: "turn-tools",
    }),
    taskComplete("turn-tools", "2026-07-16T08:05:00.000Z", 240_000, 2_000),
  ]);

  const report = await analyzeSessions({ sessionsRoot: root, sinceDays: 30, now: NOW });
  const session = report.analysis.sessions[0];

  assert.deepEqual(session.tools.summary, {
    calls: 3,
    success: 1,
    failed: 0,
    completed: 1,
    incomplete: 1,
    completionRate: 2 / 3,
  });
  assert.deepEqual(
    session.turns.items[0].tools.map((tool) => ({ name: tool.name, status: tool.status, durationMs: tool.durationMs })),
    [
      { name: "exec_command", status: "completed", durationMs: null },
      { name: "fetch_metrics", status: "success", durationMs: 2_500 },
      { name: "slow_connector", status: "incomplete", durationMs: null },
    ],
  );
  assert.equal(report.analysis.comparison.tools.topSessions[0].value, 3);
  assert.deepEqual(report.analysis.comparison.expensiveToolCalls, {
    basis: "recorded_duration",
    eligibleCalls: 1,
    topCalls: [
      {
        sessionDisplayId: session.displayId,
        turnLabel: "Turn 1",
        name: "fetch_metrics",
        durationMs: 2_500,
        status: "success",
      },
    ],
  });

  const serialized = JSON.stringify(report);
  for (const secret of ["PRIVATE_COMMAND", "PRIVATE_OUTPUT", "PRIVATE_INPUT"]) {
    assert.equal(serialized.includes(secret), false);
  }
});

test("groups spawned subagents into a workflow and compares their token footprint", async (t) => {
  const root = await fixtureRoot(t);
  await jsonl(join(root, "root.jsonl"), [
    sessionMeta("root-session", "2026-07-16T08:00:00.000Z"),
    taskStarted("root-turn", "2026-07-16T08:01:00.000Z"),
    tokenCount(100, "2026-07-16T08:02:00.000Z"),
  ]);
  await jsonl(join(root, "child.jsonl"), [
    sessionMeta("child-session", "2026-07-16T08:03:00.000Z", "/private/child", {
      parentThreadId: "root-session",
      depth: 1,
    }),
    taskStarted("child-turn", "2026-07-16T08:04:00.000Z"),
    tokenCount(40, "2026-07-16T08:05:00.000Z"),
  ]);
  await jsonl(join(root, "grandchild.jsonl"), [
    sessionMeta("grandchild-session", "2026-07-16T08:06:00.000Z", "/private/grandchild", {
      parentThreadId: "child-session",
      depth: 2,
    }),
    taskStarted("grandchild-turn", "2026-07-16T08:07:00.000Z"),
    tokenCount(30, "2026-07-16T08:08:00.000Z"),
  ]);

  const report = await analyzeSessions({ sessionsRoot: root, sinceDays: 30, now: NOW });
  const rootSession = report.analysis.sessions.find((session) => session.orchestration.role === "root");

  assert.deepEqual(report.analysis.comparison.orchestration, {
    workflows: 1,
    workflowsWithSubagents: 1,
    spawnedSubagents: 2,
    maxDepth: 2,
    rootTokens: 100,
    subagentTokens: 70,
    subagentTokenShare: 70 / 170,
    topWorkflows: [
      {
        rootDisplayId: rootSession.displayId,
        totalTokens: 170,
        subagentTokens: 70,
        spawnedSubagents: 2,
        maxDepth: 2,
      },
    ],
  });
  assert.equal(rootSession.orchestration.directSubagents, 1);
  assert.equal(rootSession.orchestration.spawnedSubagents, 2);
  assert.equal(rootSession.orchestration.totalWorkflowTokens, 170);
  assert.deepEqual(
    rootSession.orchestration.children.map((child) => ({ depth: child.depth, tokenUsage: child.tokenUsage })),
    [
      { depth: 1, tokenUsage: 40 },
      { depth: 2, tokenUsage: 30 },
    ],
  );

  const serialized = JSON.stringify(report);
  for (const secret of ["root-session", "child-session", "grandchild-session", "/private/child"]) {
    assert.equal(serialized.includes(secret), false);
  }
});

async function fixtureRoot(t) {
  const root = await mkdtemp(join(tmpdir(), "codex-inspector-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  return root;
}

async function jsonl(path, records) {
  await writeFile(path, `${records.map((record) => JSON.stringify(record)).join("\n")}\n`, "utf8");
}

function sessionMeta(id, timestamp, cwd = "/tmp/project", options = {}) {
  return {
    type: "session_meta",
    timestamp,
    payload: {
      id,
      session_id: id,
      timestamp,
      cwd,
      parent_thread_id: options.parentThreadId,
      source: options.parentThreadId
        ? {
            subagent: {
              thread_spawn: {
                parent_thread_id: options.parentThreadId,
                depth: options.depth,
                agent_path: "PRIVATE_AGENT_PATH",
              },
            },
          }
        : "cli",
    },
  };
}

function taskStarted(turnId, timestamp) {
  return {
    type: "event_msg",
    timestamp,
    payload: { type: "task_started", turn_id: turnId, started_at: timestamp },
  };
}

function tokenCount(total, timestamp, usage = {}) {
  const input = usage.input ?? total;
  const cachedInput = usage.cachedInput ?? 0;
  const output = usage.output ?? 0;
  const reasoningOutput = usage.reasoningOutput ?? 0;
  const lastTotal = usage.lastTotal ?? total;
  return {
    type: "event_msg",
    timestamp,
    payload: {
      type: "token_count",
      info: {
        total_token_usage: {
          total_tokens: total,
          input_tokens: usage.totalInput ?? input,
          cached_input_tokens: usage.totalCachedInput ?? cachedInput,
          output_tokens: usage.totalOutput ?? output,
        },
        last_token_usage: {
          total_tokens: lastTotal,
          input_tokens: input,
          cached_input_tokens: cachedInput,
          output_tokens: output,
          reasoning_output_tokens: reasoningOutput,
        },
      },
    },
  };
}

function taskComplete(turnId, timestamp, durationMs, timeToFirstTokenMs) {
  return {
    type: "event_msg",
    timestamp,
    payload: {
      type: "task_complete",
      turn_id: turnId,
      completed_at: timestamp,
      duration_ms: durationMs,
      time_to_first_token_ms: timeToFirstTokenMs,
    },
  };
}

function compaction(timestamp) {
  return { type: "event_msg", timestamp, payload: { type: "context_compacted" } };
}

function functionCall(callId, args, timestamp, options = {}) {
  return {
    type: "response_item",
    timestamp,
    payload: {
      type: "function_call",
      call_id: callId,
      name: options.name || "shell",
      arguments: args,
      internal_chat_message_metadata_passthrough: options.turnId ? { turn_id: options.turnId } : undefined,
    },
  };
}

function customToolCall(callId, input, timestamp, options = {}) {
  return {
    type: "response_item",
    timestamp,
    payload: {
      type: "custom_tool_call",
      call_id: callId,
      name: options.name || "tool",
      input,
      internal_chat_message_metadata_passthrough: options.turnId ? { turn_id: options.turnId } : undefined,
    },
  };
}

function functionCallOutput(callId, output, timestamp) {
  return {
    type: "response_item",
    timestamp,
    payload: { type: "function_call_output", call_id: callId, output },
  };
}

function mcpToolEnd(callId, timestamp, durationMs, isError) {
  return {
    type: "event_msg",
    timestamp,
    payload: {
      type: "mcp_tool_call_end",
      call_id: callId,
      duration: {
        secs: Math.floor(durationMs / 1_000),
        nanos: (durationMs % 1_000) * 1_000_000,
      },
      result: { Ok: { isError } },
    },
  };
}

function userMessage(message, timestamp) {
  return { type: "event_msg", timestamp, payload: { type: "user_message", message } };
}
