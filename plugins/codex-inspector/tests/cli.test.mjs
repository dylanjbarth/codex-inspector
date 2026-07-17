import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import test from "node:test";

const execFileAsync = promisify(execFile);
const CLI_PATH = fileURLToPath(new URL("../src/cli.mjs", import.meta.url));

test("direct CLI execution writes the fallback report to the original invocation directory", async (t) => {
  const root = await mkdtemp(join(tmpdir(), "codex-inspector-cli-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const sessionsRoot = join(root, "sessions");
  await mkdir(sessionsRoot);
  const timestamp = new Date().toISOString();
  const session = {
    type: "session_meta",
    timestamp,
    payload: { id: "cli-private-session-id", session_id: "cli-private-session-id", timestamp, cwd: "/private/project" },
  };
  const turn = {
    type: "event_msg",
    timestamp,
    payload: { type: "task_started", turn_id: "cli-turn", started_at: timestamp },
  };
  await writeFile(
    join(sessionsRoot, "session.jsonl"),
    `${JSON.stringify(session)}\n${JSON.stringify(turn)}\n`,
    "utf8",
  );

  const { stdout } = await execFileAsync(process.execPath, [CLI_PATH, "--sessions", "sessions", "--no-open"], {
    env: { ...process.env, INIT_CWD: root },
  });

  const outputPath = join(root, "codex-inspector-report.html");
  const html = await readFile(outputPath, "utf8");
  assert.match(stdout, new RegExp(`Report written to ${escapeRegExp(outputPath)}`));
  assert.match(html, /window\.__CODEX_INSPECTOR_REPORT__/);
  assert.equal(html.includes("cli-private-session-id"), false);
  assert.equal(html.includes("/private/project"), false);
});

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
