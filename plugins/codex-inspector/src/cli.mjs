#!/usr/bin/env node

import { lstat, mkdir, rename, writeFile } from "node:fs/promises";
import { platform } from "node:os";
import { dirname, resolve } from "node:path";
import { spawn } from "node:child_process";
import { pathToFileURL } from "node:url";

import { analyzeSessions, defaultSessionsRoot, InspectorError } from "./analyzer.mjs";
import { renderStandaloneReport } from "./report-html.mjs";

export async function runCli(argv = process.argv.slice(2)) {
  const options = parseArgs(argv);
  const invocationRoot = process.env.INIT_CWD || process.cwd();
  const sessionsRoot = options.sessions ? resolve(invocationRoot, options.sessions) : defaultSessionsRoot();
  const outputPath = options.output
    ? resolve(invocationRoot, options.output)
    : resolve(invocationRoot, "codex-inspector-report.html");

  process.stdout.write(
    `Analyzing local Codex metadata from the last ${options.sinceDays} days. ` +
      "Prompts, responses, source code, tool arguments, tool results, and project paths are excluded.\n",
  );

  const report = await analyzeSessions({ sessionsRoot, sinceDays: options.sinceDays });
  const html = await renderStandaloneReport(report);
  await atomicWrite(outputPath, html);
  process.stdout.write(`Report written to ${outputPath}\n`);

  if (!options.noOpen) {
    const opened = openInBrowser(outputPath);
    if (!opened) process.stdout.write("Automatic browser opening is unavailable on this platform.\n");
  }
  return report;
}

export function parseArgs(argv) {
  const result = { sinceDays: 30, output: null, sessions: null, noOpen: false };
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (argument === "--no-open") {
      result.noOpen = true;
    } else if (argument === "--since") {
      const value = argv[++index];
      const match = /^(\d+)d$/.exec(value || "");
      if (!match) throw new InspectorError("invalid_window", "--since must use the format <positive integer>d.");
      result.sinceDays = Number(match[1]);
    } else if (argument === "--output") {
      result.output = requireValue(argv[++index], "--output");
    } else if (argument === "--sessions") {
      result.sessions = requireValue(argv[++index], "--sessions");
    } else {
      throw new InspectorError("unknown_argument", `Unknown argument: ${argument}`);
    }
  }
  return result;
}

async function atomicWrite(outputPath, content) {
  const existing = await lstat(outputPath).catch(() => null);
  if (existing && (!existing.isFile() || existing.isSymbolicLink())) {
    throw new InspectorError("unsafe_output", "Output must be a regular file, not a directory or symlink.");
  }
  await mkdir(dirname(outputPath), { recursive: true });
  const temporaryPath = `${outputPath}.${process.pid}.tmp`;
  await writeFile(temporaryPath, content, { encoding: "utf8", mode: 0o600 });
  await rename(temporaryPath, outputPath);
}

function openInBrowser(outputPath) {
  const command = platform() === "darwin" ? "open" : platform() === "linux" ? "xdg-open" : null;
  if (!command) return false;
  const child = spawn(command, [outputPath], { detached: true, stdio: "ignore", shell: false });
  child.on("error", () => {});
  child.unref();
  return true;
}

function requireValue(value, option) {
  if (!value || value.startsWith("--")) throw new InspectorError("missing_argument", `${option} requires a value.`);
  return value;
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  runCli().catch((error) => {
    process.stderr.write(`Codex Inspector: ${error.message}\n`);
    process.exitCode = 1;
  });
}
