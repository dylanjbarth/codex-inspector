import type { components } from './generated/internal-api'

export type Status = components['schemas']['Status']
export type MetricResult = components['schemas']['MetricResult']
export type FilterOptions = components['schemas']['MetricFilterOptions']
export type SessionPage = components['schemas']['SessionPage']
export type SessionMap = components['schemas']['SessionMap']
export type LedgerPage = components['schemas']['LedgerPage']
export type EvidenceChunk = components['schemas']['EvidenceChunk']
export type RecordedContext = components['schemas']['RecordedContext']

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

async function inspectorJSON<T>(path:string,message:string):Promise<T>{
  const response=await fetch(path)
  if(!response.ok){let detail='';try{const problem=await response.json() as {detail?:string;title?:string};detail=problem.detail||problem.title||''}catch{/* payload-free fallback */}throw new Error(detail||message)}
  return response.json() as Promise<T>
}
export function fetchSessions(query:string,revision:number,cursor?:string):Promise<SessionPage>{const params=new URLSearchParams({revision:String(revision),pageSize:'50'});if(query)params.set('query',query);if(cursor)params.set('cursor',cursor);return inspectorJSON(`/v1/sessions?${params}`,'Session discovery is unavailable')}
export function fetchSessionMap(sessionId:string,revision:number):Promise<SessionMap>{return inspectorJSON(`/v1/sessions/${encodeURIComponent(sessionId)}/map?revision=${revision}`,'Session map is unavailable')}
export function fetchLedger(sessionId:string,turnId:string,revision:number,cursor?:string):Promise<LedgerPage>{const params=new URLSearchParams({revision:String(revision),pageSize:'200'});if(cursor)params.set('cursor',cursor);return inspectorJSON(`/v1/sessions/${encodeURIComponent(sessionId)}/turns/${encodeURIComponent(turnId)}/ledger?${params}`,'Turn evidence is unavailable')}
export function fetchEvidence(evidenceId:string,revision:number,offset=0):Promise<EvidenceChunk>{return inspectorJSON(`/v1/evidence/${encodeURIComponent(evidenceId)}?revision=${revision}&offset=${offset}&limit=65536`,'Exact evidence is unavailable')}
export function fetchRecordedContext(evidenceId:string,revision:number):Promise<RecordedContext>{return inspectorJSON(`/v1/context/${encodeURIComponent(evidenceId)}?revision=${revision}`,'Recorded context state is unavailable')}

type InspectorEvent={event:string;data:Record<string,unknown>}
const testSubscribers=new Set<(event:InspectorEvent)=>void>()
export function dispatchInspectorEvent(event:InspectorEvent){for(const listener of testSubscribers)listener(event)}
export function subscribe(onEvent:(event:InspectorEvent)=>void):()=>void {
  const controller=new AbortController();testSubscribers.add(onEvent)
  void (async()=>{try{const response=await fetch('/v1/events',{headers:{'X-Inspector-Origin':location.origin},signal:controller.signal});if(!response.ok||!response.body)return;const reader=response.body.pipeThrough(new TextDecoderStream()).getReader();let pending='';for(;;){const {done,value}=await reader.read();if(done)break;pending+=value;let split;while((split=pending.indexOf('\n\n'))>=0){const block=pending.slice(0,split);pending=pending.slice(split+2);const line=block.split('\n').find(x=>x.startsWith('data: '));if(line)onEvent(JSON.parse(line.slice(6)) as InspectorEvent)}}}catch(error){if(!controller.signal.aborted)console.error('Inspector event stream failed',error)}})()
  return()=>{testSubscribers.delete(onEvent);controller.abort()}
}
