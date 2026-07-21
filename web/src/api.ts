import type { components } from './generated/internal-api'

export type Status = components['schemas']['Status']
export type SourceDiagnosticPage = components['schemas']['SourceDiagnosticPage']
export type MetricResult = components['schemas']['MetricResult']
export type FilterOptions = components['schemas']['MetricFilterOptions']
export type SessionPage = components['schemas']['SessionPage']
export type SessionMap = components['schemas']['SessionMap']
export type LedgerPage = components['schemas']['LedgerPage']
export type EvidenceChunk = components['schemas']['EvidenceChunk']
export type RecordedContext = components['schemas']['RecordedContext']
export type ReviewPlanRequest = components['schemas']['ReviewPlanRequest']
export type ReviewPlan = components['schemas']['ReviewPlan']
export type ReviewSummary = components['schemas']['ReviewSummary']
export type ReviewPage = components['schemas']['ReviewPage']
export type ReviewDetail = components['schemas']['ReviewDetail']

export type DashboardQuery = {
  metricKeys: string[]; timezone: string; grain: 'hour'|'day'|'week'|'month';
  start?: string; end?: string; projectIds?: string[]; models?: string[];
  reasoningEfforts?: string[]; contributionKinds?: string[]; requestedRevision?: number;
}
export async function establishSession(): Promise<Status> {
  return fetchStatus()
}

export async function fetchStatus(): Promise<Status> {
  const response = await fetch('/v1/status')
  if (!response.ok) throw new Error('The local Inspector server is unavailable')
  return response.json()
}

export function fetchSourceDiagnostics(cursor?:string):Promise<SourceDiagnosticPage>{const params=new URLSearchParams({pageSize:'200'});if(cursor)params.set('cursor',cursor);return inspectorJSON(`/v1/source-diagnostics?${params}`,'Rollout file diagnostics are unavailable')}

export async function revealSourceDiagnostic(sourceId:string):Promise<void>{
  const response=await fetch(`/v1/source-diagnostics/${encodeURIComponent(sourceId)}/reveal`,{method:'POST',headers:{'X-Inspector-Origin':location.origin}})
  if(!response.ok)throw new Error('The rollout file could not be revealed in Finder')
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
export function fetchSessionMetadata(rootIds:string[],revision:number):Promise<SessionPage>{const params=new URLSearchParams({revision:String(revision),pageSize:String(Math.max(1,Math.min(200,rootIds.length)))});for(const rootId of rootIds.slice(0,200))params.append('rootId',rootId);return inspectorJSON(`/v1/sessions?${params}`,'Session metadata is unavailable')}
export function fetchSessionMap(sessionId:string,revision:number):Promise<SessionMap>{return inspectorJSON(`/v1/sessions/${encodeURIComponent(sessionId)}/map?revision=${revision}`,'Session map is unavailable')}
export function fetchLedger(sessionId:string,turnId:string,revision:number,cursor?:string):Promise<LedgerPage>{const params=new URLSearchParams({revision:String(revision),pageSize:'200'});if(cursor)params.set('cursor',cursor);return inspectorJSON(`/v1/sessions/${encodeURIComponent(sessionId)}/turns/${encodeURIComponent(turnId)}/ledger?${params}`,'Turn evidence is unavailable')}
export function fetchEvidence(evidenceId:string,revision:number,offset=0):Promise<EvidenceChunk>{return inspectorJSON(`/v1/evidence/${encodeURIComponent(evidenceId)}?revision=${revision}&offset=${offset}&limit=65536`,'Exact evidence is unavailable')}
export function fetchRecordedContext(evidenceId:string,revision:number):Promise<RecordedContext>{return inspectorJSON(`/v1/context/${encodeURIComponent(evidenceId)}?revision=${revision}`,'Recorded context state is unavailable')}

async function inspectorMutation<T>(path:string,body:unknown,message:string):Promise<T>{
  const response=await fetch(path,{method:'POST',headers:{'content-type':'application/json','X-Inspector-Origin':location.origin},body:JSON.stringify(body)})
  if(!response.ok){let detail='';try{const problem=await response.json() as {title?:string};detail=problem.title||''}catch{/* bounded fallback */}throw new Error(detail||message)}
  return response.json() as Promise<T>
}
export function createReviewPlan(request:ReviewPlanRequest):Promise<ReviewPlan>{return inspectorMutation('/v1/review-plans',request,'Review plan could not be created')}
export function launchReview(planId:string):Promise<ReviewSummary>{return inspectorMutation('/v1/reviews',{planId,confirmed:true},'Review could not be started')}
export function fetchReviews(cursor?:string):Promise<ReviewPage>{const params=new URLSearchParams({pageSize:'50'});if(cursor)params.set('cursor',cursor);return inspectorJSON(`/v1/reviews?${params}`,'Review history is unavailable')}
export function fetchReview(reviewId:string):Promise<ReviewDetail>{return inspectorJSON(`/v1/reviews/${encodeURIComponent(reviewId)}`,'Review is unavailable')}

type InspectorEvent={event:string;data:Record<string,unknown>}
const testSubscribers=new Set<(event:InspectorEvent)=>void>()
export function dispatchInspectorEvent(event:InspectorEvent){for(const listener of testSubscribers)listener(event)}
export function subscribe(onEvent:(event:InspectorEvent)=>void):()=>void {
  const controller=new AbortController();testSubscribers.add(onEvent)
  void (async()=>{try{const response=await fetch('/v1/events',{headers:{'X-Inspector-Origin':location.origin},signal:controller.signal});if(!response.ok||!response.body)return;const reader=response.body.pipeThrough(new TextDecoderStream()).getReader();let pending='';for(;;){const {done,value}=await reader.read();if(done)break;pending+=value;let split;while((split=pending.indexOf('\n\n'))>=0){const block=pending.slice(0,split);pending=pending.slice(split+2);const line=block.split('\n').find(x=>x.startsWith('data: '));if(line)onEvent(JSON.parse(line.slice(6)) as InspectorEvent)}}}catch(error){if(!controller.signal.aborted)console.error('Inspector event stream failed',error)}})()
  return()=>{testSubscribers.delete(onEvent);controller.abort()}
}
