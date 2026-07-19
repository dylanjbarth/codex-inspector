import type { components } from './generated/internal-api'

export type Status = components['schemas']['Status']
export type MetricResult = components['schemas']['MetricResult']
export type FilterOptions = components['schemas']['MetricFilterOptions']

export type DashboardQuery = {
  metricKeys: string[]; timezone: string; grain: 'hour'|'day'|'week'|'month';
  start?: string; end?: string; projectIds?: string[]; models?: string[];
  reasoningEfforts?: string[]; contributionKinds?: string[]; requestedRevision?: number;
}
export async function establishSession(): Promise<Status> {
  const bootstrap = new URLSearchParams(location.hash.replace(/^#/, ''))
  const token = bootstrap.get('token') ?? ''
  if (token) {
	const instanceId = bootstrap.get('instanceId') ?? ''
	const protocolVersion = Number(bootstrap.get('protocolVersion'))
	if (!instanceId || protocolVersion !== 1) throw new Error('Secure startup metadata is incomplete')
    const exchange = await fetch('/v1/token/exchange', {
      method: 'POST',
      headers: { 'content-type': 'application/json', 'X-Inspector-Origin': location.origin },
      body: JSON.stringify({ token, instanceId, protocolVersion }),
    })
    if (!exchange.ok) throw new Error('Secure token exchange failed')
    history.replaceState(null, '', location.pathname + location.search)
  }
  return fetchStatus()
}

export async function fetchStatus(): Promise<Status> {
  const response = await fetch('/v1/status')
  if (!response.ok) throw new Error('Inspector session is unavailable')
  return response.json()
}

export async function fetchFilterOptions(requestedRevision?:number):Promise<FilterOptions>{
  const params=new URLSearchParams();if(requestedRevision!=null)params.set('requestedRevision',String(requestedRevision));const response=await fetch(`/v1/metrics/catalog${params.size?`?${params}`:''}`);if(!response.ok)throw new Error('Metric catalog is unavailable');const body:components['schemas']['MetricCatalog']=await response.json();return body.filterOptions
}

export async function queryMetrics(query: DashboardQuery): Promise<MetricResult> {
  const response = await fetch('/v1/metrics/query', {method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify(query)})
  if (!response.ok) throw new Error('Token & Capacity metrics are unavailable')
  return response.json()
}

type InspectorEvent={event:string;data:Record<string,unknown>}
const testSubscribers=new Set<(event:InspectorEvent)=>void>()
export function dispatchInspectorEvent(event:InspectorEvent){for(const listener of testSubscribers)listener(event)}
export function subscribe(onEvent:(event:InspectorEvent)=>void):()=>void {
  const controller=new AbortController();testSubscribers.add(onEvent)
  void (async()=>{try{const response=await fetch('/v1/events',{headers:{'X-Inspector-Origin':location.origin},signal:controller.signal});if(!response.ok||!response.body)return;const reader=response.body.pipeThrough(new TextDecoderStream()).getReader();let pending='';for(;;){const {done,value}=await reader.read();if(done)break;pending+=value;let split;while((split=pending.indexOf('\n\n'))>=0){const block=pending.slice(0,split);pending=pending.slice(split+2);const line=block.split('\n').find(x=>x.startsWith('data: '));if(line)onEvent(JSON.parse(line.slice(6)) as InspectorEvent)}}}catch(error){if(!controller.signal.aborted)console.error('Inspector event stream failed',error)}})()
  return()=>{testSubscribers.delete(onEvent);controller.abort()}
}
