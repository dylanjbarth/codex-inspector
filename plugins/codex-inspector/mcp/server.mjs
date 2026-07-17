#!/usr/bin/env node

import { readFile } from "node:fs/promises";
import { createInterface } from "node:readline";
import { dirname, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import { analyzeSessions, defaultSessionsRoot } from "../src/analyzer.mjs";

export const SERVER_NAME = "codex-inspector";
export const SERVER_VERSION = "0.2.0";
export const TOOL_NAME = "inspect_codex_sessions";
export const WIDGET_URI = `ui://widget/codex-inspector-artifact-${SERVER_VERSION}.html`;
export const WIDGET_MIME_TYPE = "text/html;profile=mcp-app";

const PLUGIN_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const WIDGET_PATH = resolve(PLUGIN_ROOT, "assets", "inspector-widget.html");

export function toolUiMeta(resourceUri = WIDGET_URI) {
  return {
    ui: { resourceUri },
    "ui/resourceUri": resourceUri,
    "openai/outputTemplate": resourceUri,
    "openai/widgetAccessible": true,
    "openai/toolInvocation/invoking": "Inspecting local Codex session metadata…",
    "openai/toolInvocation/invoked": "Codex session analysis is ready",
  };
}

export function toolDefinitions() {
  return [
    {
      name: TOOL_NAME,
      title: "Inspect Codex Sessions",
      description:
        "Analyze local Codex session metadata over a selected window and render the Codex Inspector dashboard. " +
        "Use for requests about tokens, turns, duration, compactions, tool outcomes, expensive calls, subagents, or workflow improvement. " +
        "The result excludes prompts, responses, source code, tool arguments, tool results, and project paths.",
      inputSchema: {
        type: "object",
        properties: {
          since_days: {
            type: "integer",
            minimum: 1,
            maximum: 365,
            default: 30,
            description: "Number of recent days to analyze. Defaults to 30.",
          },
        },
        additionalProperties: false,
      },
      annotations: {
        readOnlyHint: true,
        destructiveHint: false,
        idempotentHint: true,
        openWorldHint: false,
      },
      _meta: toolUiMeta(),
    },
  ];
}

export function resources() {
  return [
    {
      uri: WIDGET_URI,
      name: "codex_inspector_artifact_app",
      title: "Codex Inspector dashboard",
      description: "Interactive compare-select-explain dashboard for local Codex session metadata.",
      mimeType: WIDGET_MIME_TYPE,
      _meta: widgetResourceMeta(),
    },
  ];
}

export function createRpcHandler({
  sessionsRoot = process.env.CODEX_INSPECTOR_SESSIONS_DIR || defaultSessionsRoot(),
  now = () => new Date(),
  readWidget = () => readFile(WIDGET_PATH, "utf8"),
} = {}) {
  return async function handleRpc(message) {
    if (!message || typeof message !== "object" || Array.isArray(message)) {
      return rpcError(null, -32600, "Invalid Request");
    }
    const id = message.id;
    const method = message.method;
    const params = isObject(message.params) ? message.params : {};
    if (typeof method !== "string") return id == null ? null : rpcError(id, -32600, "Invalid Request");
    if (method.startsWith("notifications/") || method === "$/cancelRequest") return null;

    try {
      if (method === "initialize") {
        return rpcResponse(id, {
          protocolVersion: params.protocolVersion || "2024-11-05",
          capabilities: {
            tools: { listChanged: false },
            resources: { subscribe: false, listChanged: false },
          },
          serverInfo: {
            name: SERVER_NAME,
            title: "Codex Inspector",
            version: SERVER_VERSION,
            description: "Evidence-backed comparison and inspection of local Codex workflows.",
          },
          instructions:
            "Call inspect_codex_sessions for Codex usage analysis. Treat comparisons and findings as review signals, not proof of inefficiency.",
        });
      }
      if (method === "ping") return rpcResponse(id, {});
      if (method === "tools/list") return rpcResponse(id, { tools: toolDefinitions() });
      if (method === "resources/list") return rpcResponse(id, { resources: resources() });
      if (method === "resources/templates/list") return rpcResponse(id, { resourceTemplates: [] });
      if (method === "prompts/list") return rpcResponse(id, { prompts: [] });
      if (method === "resources/read") {
        if (params.uri !== WIDGET_URI) return rpcError(id, -32602, `Unknown resource: ${params.uri}`);
        return rpcResponse(id, {
          contents: [
            {
              uri: WIDGET_URI,
              mimeType: WIDGET_MIME_TYPE,
              text: await readWidget(),
              _meta: widgetResourceMeta(),
            },
          ],
        });
      }
      if (method === "tools/call") {
        if (params.name !== TOOL_NAME) return rpcError(id, -32602, `Unknown tool: ${params.name}`);
        const args = isObject(params.arguments) ? params.arguments : {};
        try {
          const report = await analyzeSessions({
            sessionsRoot,
            sinceDays: args.since_days ?? 30,
            now: now(),
          });
          return rpcResponse(id, toolResult(report));
        } catch (error) {
          return rpcResponse(id, toolError(error));
        }
      }
      return rpcError(id, -32601, `Method not found: ${method}`);
    } catch (error) {
      return rpcError(id, -32000, error instanceof Error ? error.message : String(error));
    }
  };
}

function toolResult(report) {
  const findingText = report.finding.triggered
    ? `${report.finding.affectedSessionCount} session(s) matched ${report.finding.id}.`
    : `No session matched ${report.finding.id}.`;
  return {
    content: [
      {
        type: "text",
        text:
          `${findingText} Analyzed ${report.metrics.sessions} compatible session(s) over ${report.window.days} days. ` +
          "The interactive report contains metadata only.",
      },
    ],
    structuredContent: report,
    isError: false,
    _meta: toolUiMeta(),
  };
}

function toolError(error) {
  const message = error instanceof Error ? error.message : String(error);
  return {
    content: [{ type: "text", text: `Codex Inspector could not analyze sessions: ${message}` }],
    structuredContent: {
      schemaVersion: 1,
      surface: "dashboard",
      error: { message },
    },
    isError: true,
    _meta: toolUiMeta(),
  };
}

function widgetResourceMeta() {
  return {
    "openai/widgetDescription": "Compare Codex sessions, then inspect token, turn, tool, compaction, and subagent evidence.",
    "openai/widgetPrefersBorder": false,
    "openai/widgetCSP": { connect_domains: [], resource_domains: [], frame_domains: [] },
    ui: {
      prefersBorder: false,
      csp: { connectDomains: [], resourceDomains: [], frameDomains: [] },
    },
  };
}

function rpcResponse(id, result) {
  return { jsonrpc: "2.0", id, result };
}

function rpcError(id, code, message) {
  return { jsonrpc: "2.0", id, error: { code, message } };
}

function isObject(value) {
  return value && typeof value === "object" && !Array.isArray(value);
}

export function runStdio(handleRpc = createRpcHandler()) {
  const rl = createInterface({ input: process.stdin });
  rl.on("line", async (line) => {
    const trimmed = line.trim();
    if (!trimmed) return;
    let decoded;
    try {
      decoded = JSON.parse(trimmed);
    } catch (error) {
      writeRpc(rpcError(null, -32700, `Parse error: ${error.message}`));
      return;
    }
    if (Array.isArray(decoded)) {
      const responses = [];
      for (const request of decoded) {
        const response = await handleRpc(request);
        if (response) responses.push(response);
      }
      if (responses.length) writeRpc(responses);
      return;
    }
    const response = await handleRpc(decoded);
    if (response) writeRpc(response);
  });
}

function writeRpc(message) {
  process.stdout.write(`${JSON.stringify(message)}\n`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) runStdio();
