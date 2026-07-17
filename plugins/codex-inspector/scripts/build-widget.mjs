import { build } from "esbuild";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const templatePath = resolve(ROOT, "ui", "widget-template.html");
const entryPath = resolve(ROOT, "ui", "widget.js");
const outputPath = resolve(ROOT, "assets", "inspector-widget.html");

const result = await build({
  entryPoints: [entryPath],
  bundle: true,
  format: "iife",
  platform: "browser",
  target: ["es2022"],
  minify: true,
  write: false,
});
const javascript = result.outputFiles[0].text
  .replaceAll("</script", "<\\/script")
  .replace(/[ \t]+$/gm, "");
const template = await readFile(templatePath, "utf8");
if (!template.includes("<!--WIDGET_SCRIPT-->")) {
  throw new Error("Widget template is missing <!--WIDGET_SCRIPT-->.");
}
const html = template.replace("<!--WIDGET_SCRIPT-->", () => `<script>${javascript}</script>`);
await mkdir(dirname(outputPath), { recursive: true });
await writeFile(outputPath, html, "utf8");
process.stdout.write(`Built ${outputPath}\n`);
