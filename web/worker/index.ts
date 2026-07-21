interface Env {
  ASSETS: Fetcher
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const response = await env.ASSETS.fetch(request)

    if (!response.headers.get('content-type')?.includes('text/html')) {
      return response
    }

    const origin = new URL(request.url).origin
    const html = (await response.text()).replaceAll('__SITE_ORIGIN__', origin)
    const headers = new Headers(response.headers)
    headers.delete('content-length')
    headers.set('content-type', 'text/html; charset=utf-8')

    return new Response(html, {
      status: response.status,
      statusText: response.statusText,
      headers,
    })
  },
}
