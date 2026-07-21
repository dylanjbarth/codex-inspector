import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

import {
  createRpcHandler,
  TOOL_NAME,
  toolDefinitions,
  WIDGET_MIME_TYPE,
  WIDGET_URI,
} from "../mcp/server.mjs";
import { renderStandaloneReport } from "../src/report-html.mjs";

const NOW = new Date("2026-07-17T12:00:00.000Z");

test("declares a read-only MCP tool bound to a versioned UI resource", () => {
  const [tool] = toolDefinitions();
  assert.equal(tool.name, TOOL_NAME);
  assert.equal(tool.annotations.readOnlyHint, true);
  assert.equal(tool._meta["ui/resourceUri"], WIDGET_URI);
  assert.equal(tool._meta["openai/outputTemplate"], WIDGET_URI);
});

test("stdio server starts when invoked through a relative Node entry path", async () => {
  const response = await runServerProcess({ jsonrpc: "2.0", id: 1, method: "tools/list", params: {} });
  assert.equal(response.id, 1);
  assert.equal(response.result.tools[0].name, TOOL_NAME);
});

test("serves the MCP App resource and requests fullscreen with inline fallback", async () => {
  const handleRpc = createRpcHandler();
  const response = await handleRpc({ jsonrpc: "2.0", id: 1, method: "resources/read", params: { uri: WIDGET_URI } });
  const resource = response.result.contents[0];
  assert.equal(resource.mimeType, WIDGET_MIME_TYPE);
  assert.match(resource.text, /fullscreen/);
  assert.match(resource.text, /CODEX_INSPECTOR_REPORT_DATA/);
});

test("returns metadata-only structuredContent from inspect_codex_sessions", async (t) => {
  const root = await mkdtemp(join(tmpdir(), "codex-inspector-mcp-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const records = [
    {
      type: "session_meta",
      timestamp: "2026-07-16T08:00:00.000Z",
      payload: { id: "full-private-session-id", timestamp: "2026-07-16T08:00:00.000Z", cwd: "/private/project" },
    },
    {
      type: "event_msg",
      timestamp: "2026-07-16T08:01:00.000Z",
      payload: { type: "task_started", turn_id: "turn-1", started_at: "2026-07-16T08:01:00.000Z" },
    },
    {
      type: "event_msg",
      timestamp: "2026-07-16T08:02:00.000Z",
      payload: { type: "user_message", message: "RAW_PROMPT_SENTINEL" },
    },
  ];
  await writeFile(join(root, "session.jsonl"), `${records.map(JSON.stringify).join("\n")}\n`, "utf8");
  const handleRpc = createRpcHandler({ sessionsRoot: root, now: () => NOW });

  const response = await handleRpc({
    jsonrpc: "2.0",
    id: 2,
    method: "tools/call",
    params: { name: TOOL_NAME, arguments: { since_days: 30 } },
  });

  assert.equal(response.result.isError, false);
  assert.equal(response.result.structuredContent.metrics.sessions, 1);
  assert.equal(response.result.structuredContent.analysis.sessions.length, 1);
  assert.ok(response.result.structuredContent.analysis.comparison.orchestration);
  assert.equal(response.result._meta["ui/resourceUri"], WIDGET_URI);
  const serialized = JSON.stringify(response.result.structuredContent);
  assert.equal(serialized.includes("RAW_PROMPT_SENTINEL"), false);
  assert.equal(serialized.includes("/private/project"), false);
  assert.equal(serialized.includes("full-private-session-id"), false);
});

test("does not expose a missing sessions path in MCP errors", async () => {
  const privatePath = "/private/users/example/codex-sessions";
  const handleRpc = createRpcHandler({ sessionsRoot: privatePath, now: () => NOW });
  const response = await handleRpc({
    jsonrpc: "2.0",
    id: 3,
    method: "tools/call",
    params: { name: TOOL_NAME, arguments: { since_days: 30 } },
  });

  assert.equal(response.result.isError, true);
  assert.equal(JSON.stringify(response).includes(privatePath), false);
});

test("standalone fallback injects the same report model into the same widget bundle", async () => {
  const report = {
    schemaVersion: 1,
    surface: "dashboard",
    generatedAt: NOW.toISOString(),
    window: { days: 30, label: "Last 30 days" },
    metrics: { sessions: 0, turns: 0, tokenUsage: 0, toolCalls: 0, compactions: 0 },
    finding: {
      id: "compaction_churn.v1",
      title: "Repeated compaction in a continuing session",
      triggered: false,
      affectedSessionCount: 0,
      summary: "No $& pattern found.",
      interpretation: "The rule did not trigger.",
      recommendation: "Consider a handoff when useful.",
      sessions: [],
    },
    coverage: { compatibleSessions: 0, incompatibleSessions: 0, warnings: {} },
  };
  const html = await renderStandaloneReport(report);
  assert.match(html, /window\.__CODEX_INSPECTOR_REPORT__/);
  assert.match(html, /compaction_churn\.v1/);
  assert.match(html, /No \$& pattern found\./);
  assert.doesNotMatch(html, /<!--CODEX_INSPECTOR_REPORT_DATA-->/);
  const built = await readFile(new URL("../assets/inspector-widget.html", import.meta.url), "utf8");
  assert.ok(html.length > built.length);
});

function runServerProcess(request) {
  return new Promise((resolvePromise, rejectPromise) => {
    const child = spawn(process.execPath, ["mcp/server.mjs"], {
      cwd: fileURLToPath(new URL("..", import.meta.url)),
      stdio: ["pipe", "pipe", "pipe"],
    });
    let stdout = "";
    let stderr = "";
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => {
      stdout += chunk;
    });
    child.stderr.on("data", (chunk) => {
      stderr += chunk;
    });
    const timeout = setTimeout(() => {
      child.kill();
      rejectPromise(new Error("MCP stdio server did not exit after stdin closed."));
    }, 2_000);
    child.on("error", rejectPromise);
    child.on("close", (code) => {
      clearTimeout(timeout);
      if (code !== 0) {
        rejectPromise(new Error(`MCP stdio server exited with ${code}: ${stderr}`));
        return;
      }
      const line = stdout.trim().split("\n")[0];
      resolvePromise(JSON.parse(line));
    });
    child.stdin.end(`${JSON.stringify(request)}\n`);
  });
}
