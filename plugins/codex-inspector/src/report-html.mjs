import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const PLUGIN_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const REPORT_DATA_PLACEHOLDER = "<!--CODEX_INSPECTOR_REPORT_DATA-->";

export async function renderStandaloneReport(report) {
  const template = await readFile(resolve(PLUGIN_ROOT, "assets", "inspector-widget.html"), "utf8");
  if (!template.includes(REPORT_DATA_PLACEHOLDER)) {
    throw new Error("Widget asset is missing the standalone report data placeholder.");
  }
  const serialized = JSON.stringify(report).replaceAll("<", "\\u003c").replaceAll(">", "\\u003e");
  return template.replace(
    REPORT_DATA_PLACEHOLDER,
    () => `<script>window.__CODEX_INSPECTOR_REPORT__=${serialized};</script>`,
  );
}

export { REPORT_DATA_PLACEHOLDER };
