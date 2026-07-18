import type { components } from './generated/internal-api'

export type Status = components['schemas']['Status']

export async function establishSession(): Promise<Status> {
  const bootstrap = new URLSearchParams(location.hash.replace(/^#/, ''))
  const token = bootstrap.get('token') ?? ''
  if (token) {
	const instanceId = bootstrap.get('instanceId') ?? ''
	const protocolVersion = Number(bootstrap.get('protocolVersion'))
	if (!instanceId || protocolVersion !== 1) throw new Error('Secure startup metadata is incomplete')
    const exchange = await fetch('/v1/token/exchange', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ token, instanceId, protocolVersion }),
    })
    if (!exchange.ok) throw new Error('Secure token exchange failed')
    history.replaceState(null, '', location.pathname + location.search)
  }
  const response = await fetch('/v1/status')
  if (!response.ok) throw new Error('Inspector session is unavailable')
  return response.json()
}
