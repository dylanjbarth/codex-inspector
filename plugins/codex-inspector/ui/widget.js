import { App } from "@modelcontextprotocol/ext-apps/app-with-deps";

const hostApp = new App(
  { name: "Codex Inspector", version: "0.2.0" },
  { availableDisplayModes: ["inline", "fullscreen"] },
  { autoResize: true },
);

let report = readInitialReport();
let selectedSessionId = null;
let sortMode = "tokens";
let displayMode = "inline";
const expandedTurnKeys = new Set();

hostApp.addEventListener("toolresult", (result) => applyReport(result?.structuredContent || result));
hostApp.addEventListener("hostcontextchanged", (context) => {
  displayMode = context?.displayMode || displayMode;
  document.documentElement.dataset.displayMode = displayMode;
  render();
});

window.addEventListener("openai:set_globals", (event) => {
  applyReport(event.detail?.globals?.toolOutput || event.detail?.toolOutput);
});

window.addEventListener("message", (event) => {
  if (event.source !== window.parent && event.source !== window) return;
  const payload = event.data?.structuredContent || event.data?.toolOutput || event.data;
  if (payload?.schemaVersion === 1) applyReport(payload);
});

hostApp
  .connect()
  .then(async () => {
    const context = hostApp.getHostContext?.() || {};
    displayMode = context.displayMode || displayMode;
    document.documentElement.dataset.displayMode = displayMode;
    if (context.availableDisplayModes?.includes("fullscreen") || !context.availableDisplayModes) {
      await requestFullscreen();
    }
    render();
  })
  .catch(render);

render();

function readInitialReport() {
  return window.__CODEX_INSPECTOR_REPORT__ || window.openai?.toolOutput || window.openai?.toolResponseMetadata || null;
}

function applyReport(next) {
  if (!next || typeof next !== "object") return;
  report = next;
  selectedSessionId = null;
  expandedTurnKeys.clear();
  render();
}

async function requestFullscreen() {
  try {
    const result = await hostApp.requestDisplayMode({ mode: "fullscreen" });
    displayMode = result?.mode || result?.displayMode || displayMode;
  } catch {
    try {
      const result = await window.openai?.requestDisplayMode?.({ mode: "fullscreen" });
      displayMode = result?.mode || result?.displayMode || displayMode;
    } catch {
      // The host may keep the app inline; the dashboard remains usable.
    }
  }
  document.documentElement.dataset.displayMode = displayMode;
}

function render() {
  const root = document.getElementById("app");
  root.replaceChildren();
  if (!report) {
    root.append(el("div", { className: "empty", text: "Waiting for Codex session metadata…" }));
    return;
  }
  if (report.error) {
    root.append(el("div", { className: "error", text: report.error.message || "Analysis failed." }));
    return;
  }

  const shell = el("div", { className: "shell" });
  shell.append(topbar(), summaryPulse(), signals(), sessionExplorer(), longestToolCalls(), coverageFooter());
  root.append(shell);
}

function topbar() {
  const wrapper = el("header", { className: "topbar" });
  const title = el("div");
  title.append(
    el("p", { className: "eyebrow", text: `${report.window?.label || "Selected window"} · metadata only` }),
    el("h1", { text: "Codex Inspector" }),
    el("p", { className: "dek", text: "Compare your workflows, then follow the evidence down to each turn." }),
  );
  const actions = el("div", { className: "actions" });
  const expand = el("button", { className: "button", text: "Open expanded view" });
  expand.type = "button";
  expand.hidden = displayMode === "fullscreen";
  expand.addEventListener("click", requestFullscreen);
  const ask = el("button", { className: "button primary", text: "Ask Codex about this session" });
  ask.type = "button";
  ask.addEventListener("click", askCodex);
  actions.append(expand, ask);
  wrapper.append(title, actions);
  return wrapper;
}

function summaryPulse() {
  const sessions = analysisSessions();
  const comparison = report.analysis?.comparison || {};
  const totalTurnTime = sessions.reduce((sum, session) => sum + number(session.turns?.totalDurationMs), 0);
  const toolCalls = number(comparison.tools?.calls ?? report.metrics?.toolCalls);
  const completedTools = number(comparison.tools?.completedCalls);
  const values = [
    ["Sessions", report.metrics?.sessions, "workflows observed"],
    ["Token usage", report.metrics?.tokenUsage, `${number(report.metrics?.tokenCoverage?.coveredSessions ?? comparison.tokenUsage?.eligibleSessions)} sessions covered`],
    ["Turns", report.metrics?.turns, `${formatDuration(totalTurnTime)} recorded turn time`],
    ["Tool calls", toolCalls, toolCalls ? `${formatPercent(completedTools / toolCalls)} completed` : "no calls recorded"],
    ["Compactions", report.metrics?.compactions, `${formatPercent(comparison.compactions?.rate)} of sessions`],
    ["Subagents", comparison.orchestration?.spawnedSubagents, `${formatPercent(comparison.orchestration?.subagentTokenShare)} of workflow tokens`],
  ];
  const grid = el("section", { className: "pulse", ariaLabel: "Workflow overview" });
  for (const [label, value, note] of values) {
    const card = el("div", { className: "pulse-card" });
    card.append(
      el("span", { className: "metric-label", text: label }),
      el("strong", { className: "metric-value", text: formatNumber(value) }),
      el("span", { className: "metric-note", text: note }),
    );
    grid.append(card);
  }
  return grid;
}

function signals() {
  const comparison = report.analysis?.comparison || {};
  const finding = report.finding || {};
  const wrapper = el("section", { className: "signals", ariaLabel: "Cross-session signals" });
  const heading = el("div", { className: "section-heading" });
  heading.append(
    el("div", {}, [
      el("p", { className: "eyebrow", text: "Cross-session signals" }),
      el("h2", { text: "What stands out" }),
    ]),
    el("p", { className: "section-note", text: "Benchmarks describe your own selected window, not a universal ideal." }),
  );
  wrapper.append(heading);

  const strip = el("div", { className: "signal-strip" });
  strip.append(
    signalCard(
      finding.triggered ? "Review signal" : "Compaction signal",
      finding.title || "Repeated compaction",
      `${finding.summary || "No repeated-compaction signal."} ${finding.interpretation || ""}`,
      finding.triggered ? "attention" : "quiet",
    ),
    signalCard(
      "Token benchmark",
      `${formatNumber(comparison.tokenUsage?.median)} median · ${formatNumber(comparison.tokenUsage?.p90)} P90`,
      `${number(comparison.tokenUsage?.eligibleSessions)} sessions have token evidence.`,
    ),
    signalCard(
      "Turn-time benchmark",
      `${formatDuration(comparison.turnDuration?.median)} median · ${formatDuration(comparison.turnDuration?.p90)} P90`,
      `${number(comparison.turnDuration?.eligibleTurns)} completed turns have timing evidence.`,
    ),
  );
  wrapper.append(strip);
  return wrapper;
}

function signalCard(label, title, copy, tone = "") {
  const card = el("article", { className: `signal-card ${tone}`.trim() });
  card.append(
    el("span", { className: "metric-label", text: label }),
    el("strong", { text: title }),
    el("p", { text: copy }),
  );
  return card;
}

function sessionExplorer() {
  const sessions = sortedSessions();
  if (!selectedSessionId && sessions.length) selectedSessionId = sessions[0].displayId;
  const selected = sessions.find((session) => session.displayId === selectedSessionId) || sessions[0];
  const wrapper = el("section", { className: "explorer", ariaLabel: "Session comparison and detail" });
  wrapper.append(sessionLandscape(sessions), sessionStory(selected));
  return wrapper;
}

function sessionLandscape(sessions) {
  const panel = el("div", { className: "landscape panel" });
  const heading = el("div", { className: "section-heading compact" });
  const title = el("div");
  title.append(el("p", { className: "eyebrow", text: "Compare" }), el("h2", { text: "Sessions worth a look" }));
  heading.append(title, el("span", { className: "count-pill", text: `${sessions.length} sessions` }));
  panel.append(heading, sortControls());

  const list = el("div", { className: "session-list" });
  if (!sessions.length) {
    list.append(el("div", { className: "empty", text: "No compatible session details are available in this window." }));
  } else {
    const max = Math.max(1, ...sessions.map(sortValue));
    for (const session of sessions) list.append(sessionButton(session, max));
  }
  panel.append(list);
  return panel;
}

function sortControls() {
  const controls = el("div", { className: "sort-controls", ariaLabel: "Sort sessions" });
  const choices = [
    ["tokens", "Tokens"],
    ["turnTime", "Turn time"],
    ["tools", "Tool calls"],
    ["agents", "Subagents"],
  ];
  for (const [value, label] of choices) {
    const button = el("button", { className: "sort-button", text: label });
    button.type = "button";
    button.setAttribute("aria-pressed", String(sortMode === value));
    button.addEventListener("click", () => {
      sortMode = value;
      render();
    });
    controls.append(button);
  }
  return controls;
}

function sessionButton(session, maxValue) {
  const button = el("button", { className: "session-button" });
  button.type = "button";
  button.dataset.sessionId = session.displayId;
  button.setAttribute("aria-pressed", String(session.displayId === selectedSessionId));
  const row = el("div", { className: "session-row" });
  const identity = el("div");
  identity.append(
    el("strong", { text: session.displayId }),
    el("span", { className: "session-date", text: formatDate(session.startedAt) }),
  );
  const focus = sortMetric(session);
  row.append(identity, el("span", { className: "focus-value", text: focus }));
  const track = el("span", { className: "rank-track" });
  const fill = el("span", { className: "rank-fill" });
  fill.style.width = `${Math.max(3, (sortValue(session) / maxValue) * 100)}%`;
  track.append(fill);
  const tags = el("span", { className: "session-tags" });
  for (const tag of sessionTags(session)) tags.append(el("span", { text: tag }));
  button.append(row, track, tags);
  button.addEventListener("click", () => {
    selectedSessionId = session.displayId;
    render();
  });
  return button;
}

function sessionStory(session) {
  const panel = el("article", { className: "story panel" });
  panel.dataset.testid = "session-story";
  const heading = el("div", { className: "story-heading" });
  const title = el("div");
  title.append(el("p", { className: "eyebrow", text: "Explain" }), el("h2", { text: "Session story" }));
  heading.append(title);
  panel.append(heading);
  if (!session) {
    panel.append(el("div", { className: "empty", text: "Select a session to inspect its evidence." }));
    return panel;
  }

  panel.append(storyIntro(session), tokenBreakdown(session), storyMetrics(session), turnTimeline(session), orchestrationPanel(session));
  return panel;
}

function storyIntro(session) {
  const comparison = report.analysis?.comparison || {};
  const wrapper = el("div", { className: "story-intro" });
  wrapper.append(
    el("div", { className: "story-id", text: session.displayId }),
    el("p", {
      text: `${formatNumber(session.tokenUsage?.total)} tokens vs ${formatNumber(comparison.tokenUsage?.median)} session median · ${formatNumber(session.turns?.count)} turns · ${formatDuration(session.turns?.totalDurationMs)} recorded turn time.`,
    }),
  );
  return wrapper;
}

function tokenBreakdown(session) {
  const usage = session.tokenUsage || {};
  const total = Math.max(1, number(usage.input) + number(usage.output));
  const cached = Math.min(number(usage.cachedInput), number(usage.input));
  const segments = [
    ["Fresh input", Math.max(0, number(usage.input) - cached), "fresh"],
    ["Cached input", cached, "cached"],
    ["Output", usage.output, "output"],
    ["Reasoning output", usage.reasoningOutput, "reasoning"],
  ];
  const wrapper = el("section", { className: "breakdown", ariaLabel: "Token breakdown" });
  const bar = el("div", { className: "breakdown-bar" });
  for (const [, value, tone] of segments.slice(0, 3)) {
    const segment = el("span", { className: `breakdown-segment ${tone}` });
    segment.style.width = `${(number(value) / total) * 100}%`;
    bar.append(segment);
  }
  const legend = el("div", { className: "breakdown-legend" });
  for (const [label, value, tone] of segments) {
    const item = el("span", { className: `legend-item ${tone}` });
    item.append(el("i"), el("span", { text: `${label} ${formatNumber(value)}` }));
    legend.append(item);
  }
  wrapper.append(bar, legend);
  return wrapper;
}

function storyMetrics(session) {
  const tools = session.tools?.summary || {};
  const values = [
    ["Turns", session.turns?.count],
    ["Median turn", formatDuration(session.turns?.medianDurationMs), true],
    ["Compactions", session.compactions?.count],
    ["Tool calls", tools.calls],
    ["Tool completion", formatPercent(tools.completionRate), true],
    ["Subagents", session.orchestration?.spawnedSubagents],
  ];
  const grid = el("div", { className: "story-metrics" });
  for (const [label, value, preformatted] of values) {
    const item = el("div");
    item.append(
      el("span", { className: "metric-label", text: label }),
      el("strong", { text: preformatted ? value : formatNumber(value) }),
    );
    grid.append(item);
  }
  return grid;
}

function turnTimeline(session) {
  const section = el("section", { className: "turn-section" });
  const heading = el("div", { className: "subheading" });
  heading.append(el("h3", { text: "Turn-by-turn evidence" }), el("span", { text: `${number(session.turns?.count)} turns` }));
  section.append(heading);
  const list = el("div", { className: "turn-list" });
  for (const [index, turn] of (session.turns?.items || []).entries()) {
    list.append(turnCard(session, turn, index));
  }
  if (!session.turns?.items?.length) list.append(el("div", { className: "empty compact", text: "No turn-level evidence available." }));
  section.append(list);
  return section;
}

function turnCard(session, turn, index) {
  const turnKey = `${session.displayId}:${index}`;
  const expanded = expandedTurnKeys.has(turnKey);
  const detailsId = `turn-details-${safeDomId(session.displayId)}-${index}`;
  const card = el("article", { className: "turn-card" });
  if (expanded) card.classList.add("expanded");
  const toggle = el("button", { className: "turn-toggle" });
  toggle.type = "button";
  toggle.dataset.turnToggle = "";
  toggle.setAttribute("aria-expanded", String(expanded));
  toggle.setAttribute("aria-controls", detailsId);
  toggle.setAttribute("aria-label", `${expanded ? "Collapse" : "Expand"} ${turn.label} details`);
  const heading = el("div", { className: "turn-heading" });
  heading.append(
    el("span", { className: "turn-title", text: turn.label }),
    el("span", { className: `status ${statusTone(turn.status)}`, text: turn.status || "incomplete" }),
    el("span", { className: "turn-chevron", text: "⌄" }),
  );
  const facts = el("div", { className: "turn-facts" });
  const values = [
    `${formatNumber(turn.tokenUsage?.total)} tokens`,
    formatDuration(turn.durationMs),
    `${formatDuration(turn.timeToFirstTokenMs)} to first token`,
    `${number(turn.compactions)} compactions`,
    `${formatNumber(turn.tools?.length)} tool calls`,
  ];
  for (const value of values) facts.append(el("span", { text: value }));
  toggle.append(heading, facts);
  toggle.addEventListener("click", () => {
    if (expandedTurnKeys.has(turnKey)) expandedTurnKeys.delete(turnKey);
    else expandedTurnKeys.add(turnKey);
    render();
  });
  card.append(toggle);
  const details = el("div", { className: "turn-details" });
  details.id = detailsId;
  details.hidden = !expanded;
  if (turn.tools?.length) {
    const tools = el("div", { className: "tool-list" });
    for (const tool of turn.tools) {
      const item = el("div", { className: "tool-item" });
      item.append(
        el("span", { className: "tool-name", text: tool.name }),
        el("span", { className: `status ${statusTone(tool.status)}`, text: tool.status }),
        el("span", { className: "tool-duration", text: tool.durationMs == null ? "duration unavailable" : formatDuration(tool.durationMs) }),
      );
      tools.append(item);
    }
    details.append(tools);
  } else {
    details.append(el("p", { className: "no-tools", text: "No tool calls recorded for this turn." }));
  }
  card.append(details);
  return card;
}

function orchestrationPanel(session) {
  const orchestration = session.orchestration || {};
  const section = el("section", { className: "orchestration" });
  const heading = el("div", { className: "subheading" });
  heading.append(
    el("h3", { text: "Agent orchestration" }),
    el("span", { text: `${formatNumber(orchestration.spawnedSubagents)} spawned · depth ${formatNumber(orchestration.maxDepth)}` }),
  );
  section.append(heading);
  const share = number(orchestration.totalWorkflowTokens)
    ? number(orchestration.subagentTokens) / number(orchestration.totalWorkflowTokens)
    : 0;
  section.append(
    el("p", {
      className: "orchestration-copy",
      text: orchestration.spawnedSubagents
        ? `Subagents used ${formatNumber(orchestration.subagentTokens)} tokens (${formatPercent(share)}) across this workflow.`
        : "No child sessions were linked to this workflow.",
    }),
  );
  if (orchestration.children?.length) {
    const tree = el("div", { className: "agent-tree" });
    for (const child of orchestration.children) {
      const item = el("div", { className: "agent-node" });
      item.style.setProperty("--depth", String(Math.max(1, number(child.depth))));
      item.append(
        el("span", { text: child.displayId }),
        el("span", { text: `${formatNumber(child.tokenUsage)} tokens · ${formatNumber(child.turns)} turns · ${formatNumber(child.toolCalls)} tools` }),
      );
      tree.append(item);
    }
    section.append(tree);
  }
  return section;
}

function longestToolCalls() {
  const calls = report.analysis?.comparison?.expensiveToolCalls?.topCalls || [];
  const section = el("section", { className: "tool-ranking panel" });
  const heading = el("div", { className: "section-heading compact" });
  const title = el("div");
  title.append(el("p", { className: "eyebrow", text: "Measured duration" }), el("h2", { text: "Longest measured tool calls" }));
  heading.append(title, el("span", { className: "section-note", text: "Duration, not token cost" }));
  section.append(heading);
  if (!calls.length) {
    section.append(el("div", { className: "empty compact", text: "No tool calls with recorded duration in this window." }));
    return section;
  }
  const rows = el("div", { className: "ranking-rows" });
  for (const call of calls.slice(0, 8)) {
    const row = el("div", { className: "ranking-row" });
    row.append(
      el("strong", { text: call.name }),
      el("span", { text: `${call.sessionDisplayId} · ${call.turnLabel}` }),
      el("span", { className: `status ${statusTone(call.status)}`, text: call.status }),
      el("span", { className: "ranking-value", text: formatDuration(call.durationMs) }),
    );
    rows.append(row);
  }
  section.append(rows);
  return section;
}

function coverageFooter() {
  const coverage = report.coverage || {};
  const warnings = Object.values(coverage.warnings || {}).reduce((sum, value) => sum + number(value), 0);
  const footer = el("footer", { className: "coverage" });
  footer.append(
    el("span", { text: `${number(coverage.compatibleSessions)} compatible sessions · ${number(coverage.incompatibleSessions)} excluded · ${warnings} parser/coverage warnings` }),
    el("span", { text: "Prompts, responses, code, tool payloads, and project paths excluded" }),
  );
  return footer;
}

function analysisSessions() {
  return Array.isArray(report.analysis?.sessions) ? report.analysis.sessions : [];
}

function sortedSessions() {
  return [...analysisSessions()].sort((a, b) => sortValue(b) - sortValue(a) || String(a.displayId).localeCompare(String(b.displayId)));
}

function sortValue(session) {
  if (sortMode === "turnTime") return number(session.turns?.totalDurationMs);
  if (sortMode === "tools") return number(session.tools?.summary?.calls);
  if (sortMode === "agents") return number(session.orchestration?.spawnedSubagents);
  return number(session.tokenUsage?.total);
}

function sortMetric(session) {
  if (sortMode === "turnTime") return formatDuration(session.turns?.totalDurationMs);
  if (sortMode === "tools") return `${formatNumber(session.tools?.summary?.calls)} calls`;
  if (sortMode === "agents") return `${formatNumber(session.orchestration?.spawnedSubagents)} agents`;
  return `${formatNumber(session.tokenUsage?.total)} tokens`;
}

function sessionTags(session) {
  const tags = [
    `${formatNumber(session.turns?.count)} turns`,
    `${formatNumber(session.compactions?.count)} compact`,
    `${formatNumber(session.tools?.summary?.calls)} tools`,
  ];
  if (number(session.tools?.summary?.incomplete)) tags.push(`${formatNumber(session.tools.summary.incomplete)} incomplete`);
  if (number(session.orchestration?.spawnedSubagents)) tags.push(`${formatNumber(session.orchestration.spawnedSubagents)} subagents`);
  return tags;
}

async function askCodex() {
  const session = analysisSessions().find((item) => item.displayId === selectedSessionId);
  const prompt = session
    ? `Review Codex Inspector session ${session.displayId}. It used ${number(session.tokenUsage?.total)} tokens across ${number(session.turns?.count)} turns, with ${number(session.compactions?.count)} compactions, ${number(session.tools?.summary?.calls)} tool calls, and ${number(session.orchestration?.spawnedSubagents)} spawned subagents. Explain the strongest evidence-backed workflow improvement without calling the session inefficient.`
    : `Review this Codex Inspector metadata summary and suggest one evidence-backed workflow improvement without treating any metric as proof of inefficiency.`;
  try {
    if (typeof window.openai?.sendFollowUpMessage === "function") {
      await window.openai.sendFollowUpMessage({ prompt, title: "Review this Codex Inspector session" });
      return;
    }
    await hostApp.sendMessage({ role: "user", content: [{ type: "text", text: prompt }] });
  } catch {
    // The dashboard remains useful when app-originated messages are unavailable.
  }
}

function el(tag, options = {}, children = []) {
  const node = document.createElement(tag);
  if (options.className) node.className = options.className;
  if (options.text != null) node.textContent = String(options.text);
  if (options.ariaLabel) node.setAttribute("aria-label", options.ariaLabel);
  for (const child of children) node.append(child);
  return node;
}

function number(value) {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function formatNumber(value) {
  return new Intl.NumberFormat(undefined, { notation: "compact", maximumFractionDigits: 1 }).format(number(value));
}

function formatPercent(value) {
  if (value == null || !Number.isFinite(Number(value))) return "—";
  return new Intl.NumberFormat(undefined, { style: "percent", maximumFractionDigits: 0 }).format(Number(value));
}

function formatDuration(value) {
  const milliseconds = number(value);
  if (!milliseconds) return "—";
  if (milliseconds < 1_000) return `${Math.round(milliseconds)}ms`;
  if (milliseconds < 60_000) return `${(milliseconds / 1_000).toFixed(milliseconds < 10_000 ? 1 : 0)}s`;
  const minutes = Math.floor(milliseconds / 60_000);
  const seconds = Math.round((milliseconds % 60_000) / 1_000);
  return seconds ? `${minutes}m ${seconds}s` : `${minutes}m`;
}

function formatDate(value) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "Unknown date" : new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric" }).format(date);
}

function statusTone(status) {
  if (status === "success" || status === "completed") return "good";
  if (status === "failed") return "bad";
  return "pending";
}

function safeDomId(value) {
  return String(value || "session").replace(/[^a-zA-Z0-9_-]/g, "-");
}
