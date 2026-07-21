import { createServer } from 'node:http'
import { readFile } from 'node:fs/promises'

const file = '/Users/dylanbarth/Code/personal/codex-inspector/output/playwright/context-map-comparison.html'
createServer(async (_request, response) => {
  response.setHeader('content-type', 'text/html; charset=utf-8')
  response.end(await readFile(file))
}).listen(4179, '127.0.0.1')
