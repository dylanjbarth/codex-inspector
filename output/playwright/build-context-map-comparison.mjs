import { readFile, writeFile } from 'node:fs/promises'

const source = await readFile('/Users/dylanbarth/Desktop/SCR-20260720-tkbz.png')
const implementation = await readFile('/Users/dylanbarth/Code/personal/codex-inspector/output/playwright/context-map-turns-pass-2.png')
const html = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Context map comparison</title><style>body{margin:0;padding:20px;background:#222;color:white;font:14px system-ui,sans-serif}main{display:grid;grid-template-columns:1fr 1fr;gap:20px;align-items:start}figure{margin:0}figcaption{margin-bottom:8px;font-weight:700}img{display:block;width:100%;height:auto;background:white}</style></head><body><main><figure><figcaption>Wireframe source</figcaption><img src="data:image/png;base64,${source.toString('base64')}" alt="Wireframe source"></figure><figure><figcaption>Rendered implementation</figcaption><img src="data:image/png;base64,${implementation.toString('base64')}" alt="Rendered implementation"></figure></main></body></html>`
await writeFile('/Users/dylanbarth/Code/personal/codex-inspector/output/playwright/context-map-comparison.html', html)
