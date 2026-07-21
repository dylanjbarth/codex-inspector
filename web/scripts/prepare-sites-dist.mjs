import { mkdir, writeFile } from 'node:fs/promises'
import { URL } from 'node:url'

await mkdir(new URL('../dist/server/', import.meta.url), { recursive: true })
await writeFile(new URL('../dist/server/index.js', import.meta.url), `export default {
  async fetch(request, env) {
    const url = new URL(request.url)
    let response = await env.ASSETS.fetch(request)
    if (request.method === 'GET' && (response.status === 404 || response.headers.get('content-type')?.includes('text/html'))) {
      const indexRequest = new Request(new URL('/index.html', url.origin), request)
      const indexResponse = await env.ASSETS.fetch(indexRequest)
      const html = (await indexResponse.text()).replaceAll('__SITE_ORIGIN__', url.origin)
      const headers = new Headers(indexResponse.headers)
      headers.delete('content-length')
      headers.set('content-type', 'text/html; charset=utf-8')
      response = new Response(html, { status: 200, headers })
    }
    return response
  }
}\n`)
