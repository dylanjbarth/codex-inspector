import { createHash } from "node:crypto";
import { createReadStream } from "node:fs";
import { opendir, stat } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";
import { createInterface } from "node:readline";

const DAY_MS = 24 * 60 * 60 * 1000;
const MAX_DETAIL_SESSIONS = 20;
const MAX_SUMMARY_SESSIONS = 100;
const MAX_TIMELINE_COMPACTIONS = 50;
const MAX_RANKED_SESSIONS = 10;

export class InspectorError extends Error {
  constructor(code, message) {
    super(message);
    this.name = "InspectorError";
    this.code = code;
  }
}

export function defaultSessionsRoot() {
  return join(homedir(), ".codex", "sessions");
}

export async function analyzeSessions({
  sessionsRoot = defaultSessionsRoot(),
  sinceDays = 30,
  now = new Date(),
} = {}) {
  validateSinceDays(sinceDays);
  const end = validDate(now, "now");
  const start = new Date(end.getTime() - sinceDays * DAY_MS);

  const rootStats = await stat(sessionsRoot).catch(() => null);
  if (!rootStats?.isDirectory()) {
    throw new InspectorError(
      "sessions_root_missing",
      "Codex sessions directory was not found.",
    );
  }

  let files;
  try {
    files = await discoverJsonlFiles(sessionsRoot);
  } catch {
    throw new InspectorError("sessions_scan_failed", "Codex sessions directory could not be scanned.");
  }
  const warnings = freshWarnings();
  const parsedFiles = [];

  for (const file of files) {
    try {
      parsedFiles.push(await parseSessionFile(file, warnings));
    } catch {
      warnings.unreadableFiles += 1;
    }
  }

  const sessions = mergeParsedFiles(parsedFiles, warnings);
  const recognizableInWindow = [...sessions.values()].filter((session) =>
    isWithinWindow(session.startedAt, start, end),
  );
  const compatible = recognizableInWindow.filter((session) => session.turns.size > 0);
  const incompatibleSessions = recognizableInWindow.length - compatible.length;

  if (recognizableInWindow.length > 0 && compatible.length === 0) {
    throw new InspectorError(
      "unsupported_schema",
      "Codex sessions were found, but none contained a supported task_started event.",
    );
  }

  const metricAccumulator = {
    sessions: compatible.length,
    turns: 0,
    tokenUsage: 0,
    toolCalls: 0,
    compactions: 0,
    sessionsWithTokenData: 0,
  };
  const affected = [];
  let temporallyExcludedSessions = 0;

  for (const session of compatible) {
    metricAccumulator.turns += session.turns.size;
    metricAccumulator.toolCalls += session.tools.size;
    metricAccumulator.compactions += session.compactions.size;
    if (session.maxTokenUsage != null) {
      metricAccumulator.sessionsWithTokenData += 1;
      metricAccumulator.tokenUsage += session.maxTokenUsage;
    }

    const finding = findingForSession(session);
    if (finding.temporallyExcluded) temporallyExcludedSessions += 1;
    if (finding.affected) affected.push(toAffectedSession(session, finding));
  }

  affected.sort(compareAffectedSessions);
  const detailedSessions = affected.slice(0, MAX_DETAIL_SESSIONS);
  const analysis = buildSessionAnalysis(compatible);

  return {
    schemaVersion: 1,
    surface: "dashboard",
    generatedAt: end.toISOString(),
    window: {
      days: sinceDays,
      start: start.toISOString(),
      end: end.toISOString(),
      label: `Last ${sinceDays} days`,
    },
    metrics: {
      sessions: metricAccumulator.sessions,
      turns: metricAccumulator.turns,
      tokenUsage: metricAccumulator.tokenUsage,
      toolCalls: metricAccumulator.toolCalls,
      compactions: metricAccumulator.compactions,
      tokenCoverage: {
        coveredSessions: metricAccumulator.sessionsWithTokenData,
        eligibleSessions: metricAccumulator.sessions,
      },
    },
    finding: {
      id: "compaction_churn.v1",
      title: "Repeated compaction in a continuing session",
      triggered: affected.length > 0,
      affectedSessionCount: affected.length,
      summary:
        affected.length > 0
          ? `${affected.length} session${affected.length === 1 ? "" : "s"} continued after a first compaction and later reached another compaction.`
          : `No repeated-compaction pattern was found in the last ${sinceDays} days.`,
      interpretation:
        affected.length > 0
          ? "This is a pattern worth reviewing, not proof that the sessions were inefficient."
          : "This rule did not trigger; it does not mean every workflow was optimal.",
      recommendation:
        "If a session reaches a second compaction, consider a clean handoff before the next turn.",
      sessions: detailedSessions,
      displayedSessionCount: detailedSessions.length,
      summarySessionCount: Math.min(affected.length, MAX_SUMMARY_SESSIONS),
      truncated: affected.length > detailedSessions.length,
    },
    analysis,
    coverage: {
      filesScanned: files.length,
      compatibleSessions: compatible.length,
      incompatibleSessions,
      temporallyExcludedSessions,
      warnings,
    },
    privacy: {
      included: [
        "session counts",
        "turn counts",
        "cumulative token totals",
        "turn and tool durations",
        "tool names, categories, and completion states",
        "compaction timestamps",
        "parent-child session relationships",
      ],
      excluded: [
        "prompts",
        "responses",
        "source code",
        "tool arguments and results",
        "project paths",
      ],
    },
  };
}

function validateSinceDays(value) {
  if (!Number.isInteger(value) || value < 1 || value > 365) {
    throw new InspectorError("invalid_window", "sinceDays must be an integer between 1 and 365.");
  }
}

function validDate(value, label) {
  const date = value instanceof Date ? new Date(value) : new Date(value);
  if (Number.isNaN(date.getTime())) {
    throw new InspectorError("invalid_date", `${label} must be a valid date.`);
  }
  return date;
}

async function discoverJsonlFiles(root) {
  const result = [];
  const directories = [root];
  while (directories.length) {
    const directory = directories.pop();
    const entries = await opendir(directory);
    for await (const entry of entries) {
      const path = join(directory, entry.name);
      if (entry.isDirectory()) directories.push(path);
      if (entry.isFile() && entry.name.endsWith(".jsonl")) result.push(path);
    }
  }
  result.sort();
  return result;
}

async function parseSessionFile(path, warnings) {
  const parsed = {
    sessionId: null,
    parentThreadId: null,
    agentDepth: 0,
    startedAt: null,
    lastObservedAt: null,
    turns: [],
    tools: [],
    toolUpdates: [],
    compactions: [],
    tokenSnapshots: [],
  };
  const lines = createInterface({ input: createReadStream(path), crlfDelay: Infinity });

  for await (const rawLine of lines) {
    if (!rawLine.trim()) continue;
    let record;
    try {
      record = JSON.parse(rawLine);
    } catch {
      warnings.malformedLines += 1;
      continue;
    }
    if (!record || typeof record !== "object") continue;

    const topTimestamp = parseTimestamp(record.timestamp);
    parsed.lastObservedAt = maxTimestamp(parsed.lastObservedAt, topTimestamp);

    if (record.type === "session_meta") {
      const payload = asObject(record.payload);
      parsed.sessionId = stringValue(payload.id) || stringValue(payload.session_id) || parsed.sessionId;
      parsed.parentThreadId =
        stringValue(payload.parent_thread_id) ||
        stringValue(payload.source?.subagent?.thread_spawn?.parent_thread_id) ||
        parsed.parentThreadId;
      parsed.agentDepth =
        asFiniteNumber(payload.source?.subagent?.thread_spawn?.depth) ?? parsed.agentDepth;
      parsed.startedAt =
        parseTimestamp(payload.timestamp) || topTimestamp || parsed.startedAt;
      continue;
    }

    if (record.type === "event_msg") {
      const payload = asObject(record.payload);
      const eventType = payload.type;
      if (eventType === "task_started") {
        const turnId = stringValue(payload.turn_id);
        if (!turnId) {
          warnings.missingEventIds += 1;
          continue;
        }
        const timestamp = parseTimestamp(payload.started_at) || topTimestamp;
        if (!timestamp) warnings.missingTimestamps += 1;
        parsed.turns.push({
          turnId,
          timestamp,
          completedAt: null,
          durationMs: null,
          timeToFirstTokenMs: null,
          status: "in_progress",
        });
      } else if (eventType === "task_complete" || eventType === "turn_aborted") {
        const turnId = stringValue(payload.turn_id);
        if (!turnId) {
          warnings.missingEventIds += 1;
          continue;
        }
        parsed.turns.push({
          turnId,
          timestamp: null,
          completedAt: parseTimestamp(payload.completed_at) || topTimestamp,
          durationMs: asFiniteNumber(payload.duration_ms),
          timeToFirstTokenMs: asFiniteNumber(payload.time_to_first_token_ms),
          status: eventType === "task_complete" ? "completed" : "aborted",
        });
      } else if (eventType === "token_count") {
        const totalUsage = parseTokenUsage(payload.info?.total_token_usage);
        const lastUsage = parseTokenUsage(payload.info?.last_token_usage);
        if (totalUsage.total != null || lastUsage.total != null) {
          parsed.tokenSnapshots.push({
            total: totalUsage.total,
            totalUsage,
            lastUsage,
            timestamp: topTimestamp,
          });
        }
      } else if (eventType === "context_compacted") {
        if (!topTimestamp) warnings.missingTimestamps += 1;
        parsed.compactions.push({
          identity: sha256(rawLine),
          timestamp: topTimestamp,
        });
      } else if (eventType === "mcp_tool_call_end") {
        const callId = stringValue(payload.call_id);
        if (!callId) {
          warnings.missingEventIds += 1;
          continue;
        }
        const isError =
          typeof payload.result?.Ok?.isError === "boolean" ? payload.result.Ok.isError : null;
        parsed.toolUpdates.push({
          callId,
          status: isError == null ? "completed" : isError ? "failed" : "success",
          durationMs: durationToMilliseconds(payload.duration),
          timestamp: topTimestamp,
        });
      } else if (eventType === "patch_apply_end") {
        const callId = stringValue(payload.call_id);
        if (!callId) continue;
        parsed.toolUpdates.push({
          callId,
          status: payload.success === true ? "success" : payload.success === false ? "failed" : "completed",
          durationMs: null,
          timestamp: topTimestamp,
        });
      }
      continue;
    }

    if (record.type === "response_item") {
      const payload = asObject(record.payload);
      const callType =
        payload.type === "function_call" || payload.type === "custom_tool_call"
          ? payload.type
          : payload.type === "function_call_output"
            ? "function_call"
            : payload.type === "custom_tool_call_output"
              ? "custom_tool_call"
              : null;
      if (!callType) continue;
      const callId = stringValue(payload.call_id) || stringValue(payload.id);
      if (!callId) {
        warnings.missingEventIds += 1;
        continue;
      }
      if (payload.type.endsWith("_output")) {
        parsed.toolUpdates.push({ callId, status: "completed", durationMs: null, timestamp: topTimestamp });
      } else {
        parsed.tools.push({
          identity: `${callType}:${callId}`,
          callId,
          name: stringValue(payload.name) || "unknown_tool",
          category: toolCategory(payload.name),
          turnId: stringValue(payload.internal_chat_message_metadata_passthrough?.turn_id),
          status: "incomplete",
          durationMs: null,
          timestamp: topTimestamp,
        });
      }
    }
  }
  return parsed;
}

function mergeParsedFiles(parsedFiles, warnings) {
  const sessions = new Map();
  for (const file of parsedFiles) {
    if (!file.sessionId || !file.startedAt) {
      warnings.unclassifiableFiles += 1;
      continue;
    }
    let session = sessions.get(file.sessionId);
    if (!session) {
      session = {
        id: file.sessionId,
        parentThreadId: file.parentThreadId,
        agentDepth: file.agentDepth,
        startedAt: file.startedAt,
        lastObservedAt: file.lastObservedAt,
        turns: new Map(),
        tools: new Map(),
        toolUpdates: [],
        compactions: new Map(),
        tokenSnapshots: [],
        maxTokenUsage: null,
      };
      sessions.set(file.sessionId, session);
    }
    session.startedAt = minTimestamp(session.startedAt, file.startedAt);
    session.parentThreadId ||= file.parentThreadId;
    session.agentDepth = Math.max(session.agentDepth, file.agentDepth);
    session.lastObservedAt = maxTimestamp(session.lastObservedAt, file.lastObservedAt);
    for (const turn of file.turns) {
      const existing = session.turns.get(turn.turnId);
      if (!existing) {
        session.turns.set(turn.turnId, { ...turn });
        continue;
      }
      existing.timestamp = minTimestamp(existing.timestamp, turn.timestamp);
      existing.completedAt = maxTimestamp(existing.completedAt, turn.completedAt);
      existing.durationMs = turn.durationMs ?? existing.durationMs;
      existing.timeToFirstTokenMs = turn.timeToFirstTokenMs ?? existing.timeToFirstTokenMs;
      if (turn.status === "aborted" || turn.status === "completed") existing.status = turn.status;
    }
    for (const tool of file.tools) {
      const existing = session.tools.get(tool.identity);
      if (!existing) session.tools.set(tool.identity, tool);
      else {
        existing.timestamp = minTimestamp(existing.timestamp, tool.timestamp);
        existing.turnId ||= tool.turnId;
        if (existing.name === "unknown_tool") existing.name = tool.name;
      }
    }
    session.toolUpdates.push(...file.toolUpdates);
    for (const compaction of file.compactions) {
      session.compactions.set(compaction.identity, compaction);
    }
    session.tokenSnapshots.push(...file.tokenSnapshots);
  }

  for (const session of sessions.values()) {
    const sorted = session.tokenSnapshots
      .filter((snapshot) => snapshot.timestamp)
      .sort((a, b) => compareTimestamps(a.timestamp, b.timestamp));
    let previous = null;
    for (const snapshot of sorted) {
      if (previous != null && snapshot.total < previous) warnings.tokenCounterDecreases += 1;
      previous = snapshot.total;
    }
    const totals = session.tokenSnapshots.map((snapshot) => snapshot.total).filter((total) => total != null);
    session.maxTokenUsage = totals.length ? Math.max(...totals) : null;
    session.totalTokenUsage = session.tokenSnapshots
      .filter((snapshot) => snapshot.total === session.maxTokenUsage)
      .at(-1)?.totalUsage ?? emptyTokenUsage();
    for (const update of session.toolUpdates) {
      const matching = [...session.tools.values()].filter((tool) => tool.callId === update.callId);
      for (const tool of matching) {
        if (toolStatusPrecedence(update.status) >= toolStatusPrecedence(tool.status)) tool.status = update.status;
        tool.durationMs = update.durationMs ?? tool.durationMs;
      }
    }
  }
  return sessions;
}

function buildSessionAnalysis(sessions) {
  const orchestrationBySession = buildOrchestrationContexts(sessions);
  const details = sessions.map((session) => toSessionAnalysis(session, orchestrationBySession.get(session.id))).sort(
    (a, b) =>
      (b.tokenUsage.total ?? -1) - (a.tokenUsage.total ?? -1) ||
      compareIsoTimestamps(b.startedAt, a.startedAt),
  );
  const tokenEligible = details.filter((session) => session.tokenUsage.total != null);
  const tokenValues = tokenEligible.map((session) => session.tokenUsage.total);
  const turnCounts = details.map((session) => session.turns.count);
  const timedTurns = details.flatMap((session) =>
    session.turns.items.map((turn) => turn.durationMs).filter((value) => value != null),
  );
  const allTools = details.flatMap((session) =>
    session.turns.items.flatMap((turn) =>
      turn.tools.map((tool) => ({
        sessionDisplayId: session.displayId,
        turnLabel: turn.label,
        ...tool,
      })),
    ),
  );
  const timedTools = allTools.filter((tool) => tool.durationMs != null);

  return {
    comparison: {
      tokenUsage: {
        eligibleSessions: tokenEligible.length,
        median: median(tokenValues),
        p90: percentileNearestRank(tokenValues, 0.9),
        topSessions: rankedSessions(tokenEligible, (session) => session.tokenUsage.total),
      },
      turns: {
        median: median(turnCounts),
        p90: percentileNearestRank(turnCounts, 0.9),
        topSessions: rankedSessions(details, (session) => session.turns.count),
      },
      compactions: {
        sessionsWithCompactions: details.filter((session) => session.compactions.count > 0).length,
        repeatedSessions: details.filter((session) => session.compactions.count > 1).length,
        rate: details.length
          ? details.filter((session) => session.compactions.count > 0).length / details.length
          : 0,
        topSessions: rankedSessions(details, (session) => session.compactions.count),
      },
      turnDuration: {
        eligibleTurns: timedTurns.length,
        median: median(timedTurns),
        p90: percentileNearestRank(timedTurns, 0.9),
        topSessions: rankedSessions(details, (session) => session.turns.totalDurationMs),
      },
      tools: {
        calls: allTools.length,
        completedCalls: allTools.filter((tool) => tool.status !== "incomplete").length,
        incompleteCalls: allTools.filter((tool) => tool.status === "incomplete").length,
        topSessions: rankedSessions(details, (session) => session.tools.summary.calls),
      },
      expensiveToolCalls: {
        basis: "recorded_duration",
        eligibleCalls: timedTools.length,
        topCalls: timedTools
          .sort((a, b) => b.durationMs - a.durationMs || a.name.localeCompare(b.name))
          .slice(0, 10)
          .map(({ sessionDisplayId, turnLabel, name, durationMs, status }) => ({
            sessionDisplayId,
            turnLabel,
            name,
            durationMs,
            status,
          })),
      },
      orchestration: summarizeOrchestration(details),
    },
    sessions: details,
  };
}

function toSessionAnalysis(session, orchestration) {
  const turns = [...session.turns.values()]
    .filter((turn) => turn.timestamp)
    .sort((a, b) => compareTimestamps(a.timestamp, b.timestamp));
  const items = turns.map((turn, index) => toTurnAnalysis(session, turn, turns[index + 1], index));
  const durations = items.map((turn) => turn.durationMs).filter((value) => value != null);
  const reasoningOutput = session.tokenSnapshots.reduce(
    (sum, snapshot) => sum + (snapshot.lastUsage.reasoningOutput ?? 0),
    0,
  );
  const tools = [...session.tools.values()];
  const completedTools = tools.filter((tool) => tool.status !== "incomplete").length;
  return {
    displayId: displaySessionId(session.id),
    startedAt: session.startedAt,
    lastObservedAt: session.lastObservedAt,
    tokenUsage: {
      total: session.maxTokenUsage,
      input: session.totalTokenUsage.input,
      cachedInput: session.totalTokenUsage.cachedInput,
      output: session.totalTokenUsage.output,
      reasoningOutput,
    },
    turns: {
      count: turns.length,
      totalDurationMs: durations.reduce((sum, value) => sum + value, 0),
      medianDurationMs: median(durations),
      p90DurationMs: percentileNearestRank(durations, 0.9),
      items,
    },
    compactions: { count: session.compactions.size },
    tools: {
      summary: {
        calls: tools.length,
        success: tools.filter((tool) => tool.status === "success").length,
        failed: tools.filter((tool) => tool.status === "failed").length,
        completed: tools.filter((tool) => tool.status === "completed").length,
        incomplete: tools.filter((tool) => tool.status === "incomplete").length,
        completionRate: tools.length ? completedTools / tools.length : null,
      },
    },
    orchestration,
  };
}

function buildOrchestrationContexts(sessions) {
  const byId = new Map(sessions.map((session) => [session.id, session]));
  const rootIdBySession = new Map();
  const depthBySession = new Map();

  for (const session of sessions) {
    const visited = new Set([session.id]);
    let current = session;
    let derivedDepth = 0;
    while (current.parentThreadId && byId.has(current.parentThreadId) && !visited.has(current.parentThreadId)) {
      visited.add(current.parentThreadId);
      current = byId.get(current.parentThreadId);
      derivedDepth += 1;
    }
    rootIdBySession.set(session.id, current.id);
    depthBySession.set(session.id, Math.max(session.agentDepth, derivedDepth));
  }

  const contexts = new Map();
  for (const session of sessions) {
    const rootId = rootIdBySession.get(session.id);
    const workflowSessions = sessions.filter((candidate) => rootIdBySession.get(candidate.id) === rootId);
    const root = byId.get(rootId) || session;
    const descendants = workflowSessions.filter((candidate) => candidate.id !== rootId);
    const directChildren = sessions.filter((candidate) => candidate.parentThreadId === session.id);
    const rootTokens = root.maxTokenUsage ?? 0;
    const subagentTokens = descendants.reduce((sum, candidate) => sum + (candidate.maxTokenUsage ?? 0), 0);
    contexts.set(session.id, {
      role: session.id === rootId ? "root" : "subagent",
      depth: depthBySession.get(session.id),
      parentDisplayId: byId.has(session.parentThreadId) ? displaySessionId(session.parentThreadId) : null,
      rootDisplayId: displaySessionId(rootId),
      directSubagents: directChildren.length,
      spawnedSubagents: descendants.length,
      maxDepth: Math.max(0, ...workflowSessions.map((candidate) => depthBySession.get(candidate.id))),
      rootTokens,
      subagentTokens,
      totalWorkflowTokens: rootTokens + subagentTokens,
      children: descendants
        .sort(
          (a, b) =>
            depthBySession.get(a.id) - depthBySession.get(b.id) ||
            compareTimestamps(a.startedAt, b.startedAt),
        )
        .map((candidate) => ({
          displayId: displaySessionId(candidate.id),
          parentDisplayId: byId.has(candidate.parentThreadId)
            ? displaySessionId(candidate.parentThreadId)
            : null,
          depth: depthBySession.get(candidate.id),
          tokenUsage: candidate.maxTokenUsage,
          turns: candidate.turns.size,
          toolCalls: candidate.tools.size,
        })),
    });
  }
  return contexts;
}

function summarizeOrchestration(details) {
  const workflows = details.filter((session) => session.orchestration.role === "root");
  const rootTokens = workflows.reduce((sum, session) => sum + session.orchestration.rootTokens, 0);
  const subagentTokens = workflows.reduce((sum, session) => sum + session.orchestration.subagentTokens, 0);
  return {
    workflows: workflows.length,
    workflowsWithSubagents: workflows.filter((session) => session.orchestration.spawnedSubagents > 0).length,
    spawnedSubagents: workflows.reduce(
      (sum, session) => sum + session.orchestration.spawnedSubagents,
      0,
    ),
    maxDepth: Math.max(0, ...workflows.map((session) => session.orchestration.maxDepth)),
    rootTokens,
    subagentTokens,
    subagentTokenShare: rootTokens + subagentTokens ? subagentTokens / (rootTokens + subagentTokens) : null,
    topWorkflows: workflows
      .sort(
        (a, b) =>
          b.orchestration.totalWorkflowTokens - a.orchestration.totalWorkflowTokens ||
          a.displayId.localeCompare(b.displayId),
      )
      .slice(0, MAX_RANKED_SESSIONS)
      .map((session) => ({
        rootDisplayId: session.displayId,
        totalTokens: session.orchestration.totalWorkflowTokens,
        subagentTokens: session.orchestration.subagentTokens,
        spawnedSubagents: session.orchestration.spawnedSubagents,
        maxDepth: session.orchestration.maxDepth,
      })),
  };
}

function toTurnAnalysis(session, turn, nextTurn, index) {
  const end = turn.completedAt || nextTurn?.timestamp || session.lastObservedAt;
  const snapshots = session.tokenSnapshots.filter((snapshot) =>
    timestampInTurn(snapshot.timestamp, turn.timestamp, end, nextTurn?.timestamp),
  );
  const tokenUsage = snapshots.reduce(
    (total, snapshot) => addTokenUsage(total, snapshot.lastUsage),
    zeroTokenUsage(),
  );
  const compactions = [...session.compactions.values()].filter((compaction) =>
    timestampInTurn(compaction.timestamp, turn.timestamp, end, nextTurn?.timestamp),
  ).length;
  const tools = [...session.tools.values()]
    .filter(
      (tool) =>
        tool.turnId === turn.turnId ||
        (!tool.turnId && timestampInTurn(tool.timestamp, turn.timestamp, end, nextTurn?.timestamp)),
    )
    .sort((a, b) => compareTimestamps(a.timestamp, b.timestamp))
    .map(({ name, category, status, durationMs }) => ({ name, category, status, durationMs }));
  return {
    label: `Turn ${index + 1}`,
    startedAt: turn.timestamp,
    completedAt: turn.completedAt,
    status: turn.status,
    durationMs: turn.durationMs,
    timeToFirstTokenMs: turn.timeToFirstTokenMs,
    tokenUsage,
    compactions,
    tools,
  };
}

function timestampInTurn(timestamp, start, end, nextStart) {
  if (!timestamp || !start) return false;
  if (timestamp < start) return false;
  if (nextStart && timestamp >= nextStart) return false;
  return !end || timestamp <= end;
}

function rankedSessions(sessions, getValue) {
  return sessions
    .map((session) => ({ displayId: session.displayId, value: getValue(session) }))
    .filter((item) => item.value != null)
    .sort((a, b) => b.value - a.value || a.displayId.localeCompare(b.displayId))
    .slice(0, MAX_RANKED_SESSIONS);
}

function findingForSession(session) {
  const compactions = [...session.compactions.values()]
    .filter((event) => event.timestamp)
    .sort((a, b) => compareTimestamps(a.timestamp, b.timestamp) || a.identity.localeCompare(b.identity));
  const turns = [...session.turns.values()].filter((event) => event.timestamp);
  const temporallyExcluded = session.compactions.size > 0 && compactions.length !== session.compactions.size;
  if (compactions.length < 2) return { affected: false, temporallyExcluded };

  const turnsAfterEach = compactions.map(
    (compaction) => turns.filter((turn) => turn.timestamp > compaction.timestamp).length,
  );
  return {
    affected: turnsAfterEach[0] > 0,
    temporallyExcluded,
    compactions,
    turnsAfterEach,
  };
}

function toAffectedSession(session, finding) {
  const compactedForTimeline = trimCompactions(finding.compactions);
  const timeline = [];
  if (session.startedAt) {
    timeline.push({ kind: "session_started", label: "Session started", timestamp: session.startedAt });
  }
  for (const item of compactedForTimeline.items) {
    timeline.push({
      kind: "context_compacted",
      label: `Context compacted #${item.index + 1}`,
      timestamp: item.compaction.timestamp,
      turnsAfter: finding.turnsAfterEach[item.index],
    });
  }
  if (session.lastObservedAt) {
    timeline.push({
      kind: "last_recorded_event",
      label: "Last recorded event",
      timestamp: session.lastObservedAt,
    });
  }
  timeline.sort((a, b) => compareTimestamps(a.timestamp, b.timestamp) || timelinePrecedence(a) - timelinePrecedence(b));

  return {
    displayId: `session-${sha256(session.id).slice(0, 8)}`,
    startedAt: session.startedAt,
    lastObservedAt: session.lastObservedAt,
    turns: session.turns.size,
    compactions: finding.compactions.length,
    turnsAfterFirstCompaction: finding.turnsAfterEach[0],
    turnsAfterEachCompaction: finding.turnsAfterEach,
    timeline,
    timelineCompactionsOmitted: compactedForTimeline.omitted,
  };
}

function trimCompactions(compactions) {
  if (compactions.length <= MAX_TIMELINE_COMPACTIONS) {
    return { items: compactions.map((compaction, index) => ({ compaction, index })), omitted: 0 };
  }
  const first = compactions.slice(0, 25).map((compaction, index) => ({ compaction, index }));
  const last = compactions.slice(-25).map((compaction, offset) => ({
    compaction,
    index: compactions.length - 25 + offset,
  }));
  return { items: [...first, ...last], omitted: compactions.length - 50 };
}

function compareAffectedSessions(a, b) {
  return (
    b.compactions - a.compactions ||
    b.turnsAfterFirstCompaction - a.turnsAfterFirstCompaction ||
    compareTimestamps(b.startedAt, a.startedAt)
  );
}

function timelinePrecedence(event) {
  return { session_started: 0, context_compacted: 1, last_recorded_event: 2 }[event.kind] ?? 9;
}

function freshWarnings() {
  return {
    malformedLines: 0,
    unreadableFiles: 0,
    unclassifiableFiles: 0,
    missingEventIds: 0,
    missingTimestamps: 0,
    tokenCounterDecreases: 0,
  };
}

function isWithinWindow(value, start, end) {
  return value instanceof Date && value >= start && value <= end;
}

function parseTimestamp(value) {
  if (typeof value !== "string" && typeof value !== "number") return null;
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? null : parsed;
}

function asObject(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function stringValue(value) {
  return typeof value === "string" && value.length ? value : null;
}

function asFiniteNumber(value) {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : null;
}

function durationToMilliseconds(value) {
  const duration = asObject(value);
  const seconds = asFiniteNumber(duration.secs);
  const nanos = asFiniteNumber(duration.nanos);
  if (seconds == null && nanos == null) return null;
  return (seconds ?? 0) * 1_000 + (nanos ?? 0) / 1_000_000;
}

function toolCategory(name) {
  const value = typeof name === "string" ? name.toLowerCase() : "";
  if (/(exec|shell|terminal|stdin|command)/.test(value)) return "shell";
  if (/(patch|file|image|read|write)/.test(value)) return "file";
  if (/(search|web|browser|scrape)/.test(value)) return "research";
  if (/(agent|collaboration|thread)/.test(value)) return "agent";
  return "mcp_or_app";
}

function toolStatusPrecedence(status) {
  return { incomplete: 0, completed: 1, success: 2, failed: 2 }[status] ?? 0;
}

function parseTokenUsage(value) {
  const usage = asObject(value);
  return {
    total: asFiniteNumber(usage.total_tokens),
    input: asFiniteNumber(usage.input_tokens),
    cachedInput: asFiniteNumber(usage.cached_input_tokens),
    output: asFiniteNumber(usage.output_tokens),
    reasoningOutput: asFiniteNumber(usage.reasoning_output_tokens),
  };
}

function emptyTokenUsage() {
  return { total: null, input: null, cachedInput: null, output: null, reasoningOutput: null };
}

function zeroTokenUsage() {
  return { total: 0, input: 0, cachedInput: 0, output: 0, reasoningOutput: 0 };
}

function addTokenUsage(total, usage) {
  for (const key of Object.keys(total)) total[key] += usage[key] ?? 0;
  return total;
}

function median(values) {
  if (!values.length) return null;
  const sorted = [...values].sort((a, b) => a - b);
  const middle = Math.floor(sorted.length / 2);
  return sorted.length % 2 ? sorted[middle] : (sorted[middle - 1] + sorted[middle]) / 2;
}

function percentileNearestRank(values, percentile) {
  if (!values.length) return null;
  const sorted = [...values].sort((a, b) => a - b);
  return sorted[Math.max(0, Math.ceil(percentile * sorted.length) - 1)];
}

function displaySessionId(sessionId) {
  return `session-${sha256(sessionId).slice(0, 8)}`;
}

function compareIsoTimestamps(a, b) {
  return new Date(a).getTime() - new Date(b).getTime();
}

function compareTimestamps(a, b) {
  if (!a && !b) return 0;
  if (!a) return 1;
  if (!b) return -1;
  return a.getTime() - b.getTime();
}

function minTimestamp(a, b) {
  if (!a) return b;
  if (!b) return a;
  return a < b ? a : b;
}

function maxTimestamp(a, b) {
  if (!a) return b;
  if (!b) return a;
  return a > b ? a : b;
}

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}
