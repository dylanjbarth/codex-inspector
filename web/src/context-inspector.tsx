import React from 'react'
import {AlertTriangle,ArrowRight,Bot,ChevronDown,ChevronRight,LoaderCircle,MessageSquareText,Search,ServerCog,UserRound,Wrench} from 'lucide-react'
import {fetchEvidence,fetchLedger,fetchSessionMap,fetchSessionMetadata,fetchSessions,type EvidenceChunk,type LedgerPage,type SessionMap,type SessionPage} from './api'
import {navigate} from './navigation'

type Props={revision:number;available:number|null;onApply:()=>Promise<void>}
type Route={sessionId?:string;turnId?:string;eventId?:string;evidenceId?:string}
const largeMapTurnThreshold=500
const turnRenderBatchSize=200

function route():Route{
  const parts=location.pathname.split('/').filter(Boolean).map(value=>{try{return decodeURIComponent(value)}catch{return ''}})
  if(parts[0]!=='context')return {}
  const eventIndex=parts.indexOf('event'),turnIndex=parts.indexOf('turn')
  return {sessionId:parts[1],turnId:turnIndex>=0?parts[turnIndex+1]:undefined,eventId:eventIndex>=0?parts[eventIndex+1]:undefined,evidenceId:new URLSearchParams(location.search).get('evidence')||undefined}
}
function inspectorPath(path:string,updates:Record<string,string|undefined>={}){const current=new URLSearchParams(location.search),next=new URLSearchParams();for(const key of ['q','return']){const value=current.get(key);if(value)next.set(key,value)}for(const [key,value] of Object.entries(updates)){if(value)next.set(key,value);else next.delete(key)}return `${path}${next.size?`?${next}`:''}`}
const tokens=(value:number|null|undefined)=>value==null?'Unavailable':new Intl.NumberFormat().format(value)
const kindLabel=(value:string)=>value.replaceAll('_',' ').replace(/\b\w/g,letter=>letter.toUpperCase())
const eventLabel=(value:string)=>({task_started:'Turn started',task_complete:'Turn completed',turn_context:'Codex setup',world_state:'Available capabilities',token_count:'Token usage',source_boundary:'Session boundary',turn_stop:'Turn ended',function_call:'Tool request',custom_tool_call:'Tool request',tool_search_call:'Tool request',function_call_output:'Tool result',custom_tool_call_output:'Tool result',tool_search_output:'Tool result',compacted:'Context refresh',compaction:'Context refresh'}[value]||kindLabel(value))
const usableTitle=(value:string|undefined)=>{const title=value?.trim()||'';return title&&!/^<(environment_context|system|developer|image)\b/i.test(title)&&!/^\{["']?(cwd|shell|current_date)["']?\s*:/i.test(title)&&!/^the following is the codex agent history/i.test(title)?title:''}
const projectName=(value:string|undefined)=>{if(!value)return '';try{const url=new URL(value);return url.pathname.split('/').filter(Boolean).at(-1)||url.hostname}catch{return value.replace(/[\\/]$/,'').split(/[\\/]/).filter(Boolean).at(-1)||value}}
const sessionTitle=(item:SessionPage['items'][number])=>usableTitle(item.title)||[projectName(item.project),item.startedAt&&new Date(item.startedAt).toLocaleDateString(undefined,{month:'short',day:'numeric'})].filter(Boolean).join(' · ')||`Session ${item.sessionId.slice(-8)}`
const matchLabel=(value:string)=>{const [scope,field]=value.split(':').map(part=>part.trim());if(!field)return value==='root session'?'Recent session':kindLabel(value);return `${scope==='descendant'?'Found in delegated work':'Found in this session'} · ${field.replaceAll('_',' ')}`}
const sessionDuration=(item:SessionPage['items'][number])=>{if(!item.startedAt||!item.latestCompleted)return '';const elapsed=new Date(item.latestCompleted).getTime()-new Date(item.startedAt).getTime();if(!Number.isFinite(elapsed)||elapsed<0)return '';const minutes=Math.max(1,Math.round(elapsed/60000));return minutes<60?`${minutes} min`:`${Math.floor(minutes/60)} hr ${minutes%60?`${minutes%60} min`:''}`.trim()}
function Highlight({text,query}:{text:string;query:string}){const needles=[...new Set(query.trim().split(/\s+/).filter(Boolean).map(value=>value.toLowerCase()))];if(!needles.length)return <>{text}</>;const expression=needles.sort((a,b)=>b.length-a.length).map(value=>value.replace(/[.*+?^${}()|[\]\\]/g,'\\$&')).join('|'),bits=text.split(new RegExp(`(${expression})`,'ig'));return <>{bits.map((part,index)=>needles.includes(part.toLowerCase())?<mark key={index}>{part}</mark>:<React.Fragment key={index}>{part}</React.Fragment>)}</>}
type UnknownRecord=Record<string,unknown>
const isRecord=(value:unknown):value is UnknownRecord=>value!==null&&typeof value==='object'&&!Array.isArray(value)
const textValue=(value:unknown)=>typeof value==='string'?value:''
const numberValue=(value:unknown)=>typeof value==='number'&&Number.isFinite(value)?value:null
function parseJSON(value:string):unknown{try{return JSON.parse(value)}catch{return undefined}}
function displayJSON(value:unknown){if(typeof value==='string'){const parsed=parseJSON(value);if(parsed!==undefined)return JSON.stringify(parsed,null,2);return value}try{return JSON.stringify(value,null,2)}catch{return String(value)}}
function contentText(value:unknown){if(typeof value==='string')return value;if(!Array.isArray(value))return '';return value.map(block=>isRecord(block)?textValue(block.text)||textValue(block.output_text)||textValue(block.input_text):'').filter(Boolean).join('\n')}
function dateValue(value:unknown){const number=numberValue(value),date=number==null?new Date(textValue(value)):new Date(number<10_000_000_000?number*1000:number);return Number.isNaN(date.getTime())?'':date.toLocaleString()}
function durationValue(value:unknown){const ms=numberValue(value);if(ms==null)return '';if(ms<1000)return `${ms} ms`;const seconds=Math.round(ms/100)/10;if(seconds<60)return `${seconds} sec`;const minutes=Math.floor(seconds/60),remaining=Math.round(seconds%60);return `${minutes} min${remaining?` ${remaining} sec`:''}`}
function ScalarFields({fields}:{fields:Array<[string,unknown]>}){const visible=fields.filter(([,value])=>['string','number','boolean'].includes(typeof value)&&value!=='');if(!visible.length)return null;return <dl>{visible.map(([label,value])=><React.Fragment key={label}><dt>{label}</dt><dd>{typeof value==='number'?new Intl.NumberFormat().format(value):String(value)}</dd></React.Fragment>)}</dl>}
function DataBlock({title,value}:{title:string;value:unknown}){if(value==null||value==='')return null;return <><h4>{title}</h4><pre>{displayJSON(value)}</pre></>}
function EvidenceView({kind,observedAt,raw,complete}:{kind:string;observedAt:string;raw:string;complete:boolean}){
  const parsed=parseJSON(raw)
  if(!isRecord(parsed))return <div className="formatted-evidence"><div><span className="section-label">{eventLabel(kind)}</span><time>{new Date(observedAt).toLocaleString()}</time></div><h3>{eventLabel(kind)}</h3><p>{complete?'Inspector is showing this original record as plain text.':'Load the rest of this record to see all of its details.'}</p>{raw&&<p className="formatted-preview">{raw}</p>}</div>
  const payload=isRecord(parsed.payload)?parsed.payload:parsed,type=textValue(payload.type)||kind,label=eventLabel(type)
  const content=contentText(payload.content)||textValue(payload.message)||textValue(payload.last_agent_message)||contentText(payload.summary)
  const toolRequest=['custom_tool_call','function_call','tool_search_call'].includes(type)
  const toolResult=['custom_tool_call_output','function_call_output','tool_search_output'].includes(type)
  const toolInput=payload.input??payload.arguments
  const toolOutput=payload.output
  let description='A saved moment from this Codex turn.'
  let fields:Array<[string,unknown]> = []
  let detail:React.ReactNode=null
  if(type==='task_started'){
    description='How Codex was set up when this turn began.'
    fields=[['Model context window',payload.model_context_window],['Collaboration mode',payload.collaboration_mode_kind],['Started',dateValue(payload.started_at)],['Turn ID',payload.turn_id]]
  }else if(type==='task_complete'){
    description='When this turn finished and how long it took.'
    fields=[['Duration',durationValue(payload.duration_ms)],['Time to first token',durationValue(payload.time_to_first_token_ms)],['Completed',dateValue(payload.completed_at)],['Turn ID',payload.turn_id]]
  }else if(type==='token_count'){
    const info=isRecord(payload.info)?payload.info:{},last=isRecord(info.last_token_usage)?info.last_token_usage:{},total=isRecord(info.total_token_usage)?info.total_token_usage:{},limits=isRecord(payload.rate_limits)?payload.rate_limits:{},primary=isRecord(limits.primary)?limits.primary:{},credits=isRecord(limits.credits)?limits.credits:{}
    description='Token use and available capacity at this point in the turn.'
    fields=[['Context window',info.model_context_window],['Rate limit',limits.limit_id],['Used',numberValue(primary.used_percent)==null?'':`${primary.used_percent}%`],['Window',numberValue(primary.window_minutes)==null?'':`${primary.window_minutes} min`],['Resets',dateValue(primary.resets_at)],['Plan',limits.plan_type],['Credits',credits.balance]]
    detail=<div className="evidence-groups"><div><h4>This update</h4><ScalarFields fields={[['Input',last.input_tokens],['Cached input',last.cached_input_tokens],['Output',last.output_tokens],['Reasoning output',last.reasoning_output_tokens],['Total',last.total_tokens]]}/></div><div><h4>Turn total so far</h4><ScalarFields fields={[['Input',total.input_tokens],['Cached input',total.cached_input_tokens],['Output',total.output_tokens],['Reasoning output',total.reasoning_output_tokens],['Total',total.total_tokens]]}/></div></div>
  }else if(type==='turn_context'){
    const sandbox=isRecord(payload.sandbox_policy)?payload.sandbox_policy:isRecord(payload.file_system_sandbox_policy)?payload.file_system_sandbox_policy:{}
    description='The model, workspace, and permissions Codex used for this turn.'
    fields=[['Model',payload.model],['Reasoning effort',payload.effort],['Working directory',payload.cwd],['Current date',payload.current_date],['Timezone',payload.timezone],['Approval policy',payload.approval_policy],['Sandbox',sandbox.type??sandbox.mode],['Network access',sandbox.network_access],['Collaboration mode',isRecord(payload.collaboration_mode)?payload.collaboration_mode.mode??payload.collaboration_mode.kind:payload.collaboration_mode],['Multi-agent mode',payload.multi_agent_mode]]
    detail=<DataBlock title="Available workspace folders" value={payload.workspace_roots}/>
  }else if(type==='sub_agent_activity'){
    description='Activity from work Codex delegated to another agent.'
    fields=[['Activity',payload.kind],['Agent path',payload.agent_path],['Agent thread',payload.agent_thread_id],['Occurred',dateValue(payload.occurred_at_ms)],['Event ID',payload.event_id]]
  }else if(type==='message'||type==='user_message'||type==='agent_message'){
    description=`A ${textValue(payload.role)||type.replace('_message','')} message from this turn.`
    fields=[['Role',payload.role],['Phase',payload.phase],['Message ID',payload.id]]
  }else if(toolRequest||toolResult){
    description=toolRequest?'What Codex asked this tool to do.':'What the tool returned to Codex.'
    fields=[['Tool',payload.name],['Call ID',payload.call_id],['Status',payload.status]]
    detail=<>{toolRequest&&<DataBlock title="Tool input" value={toolInput}/>} {toolResult&&<DataBlock title="Tool result" value={toolOutput}/>}</>
  }else if(type.includes('reasoning')){
    description="A saved summary of Codex's reasoning. Private reasoning that was not recorded cannot be shown."
    fields=[['Status',payload.status],['Encrypted content',payload.encrypted_content?'Present':'']]
  }else if(type.includes('compact')){
    description='Codex refreshed its working context at this point.'
    fields=[['Window',payload.window_number],['Window ID',payload.window_id],['Previous window',payload.previous_window_id],['First window',payload.first_window_id]]
    detail=<div className="compaction-note"><strong>Context refresh recorded</strong><span>Inspector shows the totals around this moment, but does not guess what Codex kept or removed.</span></div>
  }else if(type==='world_state'){
    const state=isRecord(payload.state)?payload.state:{}
    description='The apps, skills, and environment available to Codex at this point.'
    fields=[['Apps instructions',state.apps_instructions],['Plugins instructions',state.plugins_instructions],['Skills',isRecord(state.skills)?Object.keys(state.skills).length:''],['Environments',isRecord(state.environments)?Object.keys(state.environments).length:'']]
    detail=<DataBlock title="Available capabilities" value={Object.fromEntries(Object.entries(state).slice(0,12).map(([key,value])=>[key,isRecord(value)?`${Object.keys(value).length} entries`:value]))}/>
  }else{
    description='Additional details saved during this turn.'
    fields=Object.entries(payload).filter(([key,value])=>key!=='type'&&['string','number','boolean'].includes(typeof value)).slice(0,12).map(([key,value])=>[kindLabel(key),value])
    const objects=Object.fromEntries(Object.entries(payload).filter(([,value])=>isRecord(value)||Array.isArray(value)).slice(0,4))
    if(Object.keys(objects).length)detail=<DataBlock title="Saved details" value={objects}/>
  }
  return <div className="formatted-evidence"><div><span className="section-label">{label}</span><time>{new Date(observedAt).toLocaleString()}</time></div><h3>{label}</h3><p>{description}</p><ScalarFields fields={fields}/>{content&&<p className="formatted-content">{content}</p>}{detail}</div>
}

type LedgerItem=LedgerPage['items'][number]
type LedgerActor=LedgerItem['actor']
type LedgerFilter='all'|'messages'|'tools'|'runtime'|'errors'
type LedgerGroup={id:string;actor:LedgerActor;family:LedgerItem['family'];items:LedgerItem[];label:string;detail:string;hasError:boolean}
const actorLabels:Record<LedgerActor,string>={user:'You',runtime:'Codex setup',agent:'Codex',tool:'Tool'}
const routineRuntimeKinds=new Set(['task_started','task_complete','turn_context','world_state','token_count','source_boundary','turn_stop'])

function LedgerActorIcon({actor}:{actor:LedgerActor}){
  if(actor==='user')return <UserRound aria-hidden="true"/>
  if(actor==='agent')return <Bot aria-hidden="true"/>
  if(actor==='tool')return <Wrench aria-hidden="true"/>
  return <ServerCog aria-hidden="true"/>
}
function ledgerItemLabel(item:LedgerItem){
  if(item.toolName)return item.toolName
  if(item.messageRole==='user')return 'Your message'
  if(item.messageRole==='assistant')return 'Agent response'
  if(item.messageRole==='system'||item.messageRole==='developer')return `${kindLabel(item.messageRole)} instructions`
  return eventLabel(item.kind)
}
function toolDetail(items:LedgerItem[]){
  const request=items.find(item=>item.toolPhase==='request'),result=items.find(item=>item.toolPhase==='result'),status=result?.status||request?.status
  if(result?.exitCode!=null)return `${result.exitCode===0?'Completed':'Failed'} · exit ${result.exitCode}${result.durationMs!=null?` · ${durationValue(result.durationMs)}`:''}`
  if(status)return `${kindLabel(status)}${result?.durationMs!=null?` · ${durationValue(result.durationMs)}`:''}`
  return result?'Request and result available':'Tool started · result not available'
}
function makeLedgerGroups(items:LedgerItem[]):LedgerGroup[]{
  const groups:LedgerGroup[]=[],toolGroups=new Map<string,LedgerGroup>()
  for(const item of items){
    if(item.callId){
      const existing=toolGroups.get(item.callId)
      if(existing){
        existing.items.push(item)
        existing.hasError=existing.hasError||item.family==='error'||item.exitCode!=null&&item.exitCode!==0
        existing.detail=toolDetail(existing.items)
        continue
      }
      const group:LedgerGroup={id:`tool:${item.callId}`,actor:'agent',family:'tool',items:[item],label:item.toolName||'Tool call',detail:toolDetail([item]),hasError:item.family==='error'||item.exitCode!=null&&item.exitCode!==0}
      toolGroups.set(item.callId,group);groups.push(group);continue
    }
    const previous=groups.at(-1),runtimeInput=item.actor==='runtime'&&(item.messageRole==='system'||item.messageRole==='developer'),routine=item.actor==='runtime'&&(routineRuntimeKinds.has(item.kind)||runtimeInput)
    if(routine&&previous?.actor==='runtime'&&previous.family==='runtime'){
      previous.items.push(item);previous.label=previous.items[0].kind==='turn_context'?'Codex setup':'Behind-the-scenes setup';previous.detail=`${previous.items.length} saved moments`;continue
    }
    const label=routine?(runtimeInput||item.kind==='turn_context'?'Codex setup':'Behind-the-scenes setup'):ledgerItemLabel(item),detail=item.actor==='runtime'&&routine?'1 saved moment':actorLabels[item.actor]
    groups.push({id:`event:${item.eventId}`,actor:item.actor,family:routine?'runtime':item.family,items:[item],label,detail,hasError:item.family==='error'})
  }
  return groups
}
function groupMatchesFilter(group:LedgerGroup,filter:LedgerFilter){
  if(filter==='all')return true
  if(filter==='messages')return group.family==='message'
  if(filter==='tools')return group.family==='tool'
  if(filter==='runtime')return group.actor==='runtime'
  return group.hasError
}
function rawLedgerGroups(items:LedgerItem[]):LedgerGroup[]{return items.map(item=>({id:`raw:${item.eventId}`,actor:item.actor,family:item.family,items:[item],label:eventLabel(item.kind),detail:actorLabels[item.actor],hasError:item.family==='error'||item.exitCode!=null&&item.exitCode!==0}))}

export function ContextInspector({revision,available,onApply}:Props){
  const [routeState,setRoute]=React.useState(route())
  const returnTo=new URLSearchParams(location.search).get('return')
  React.useEffect(()=>{const update=()=>setRoute(route());addEventListener('popstate',update);return()=>removeEventListener('popstate',update)},[])
  return <section className="inspector-shell"><header><h1>Session Inspector</h1><nav>{returnTo&&returnTo.startsWith('/reviews/')&&<a className="quiet-link" href={returnTo}>Back to review</a>}</nav></header>
    {routeState.sessionId&&available!=null&&available!==revision&&<button className="new-data" onClick={()=>void onApply()}>New session data available — update without losing your place</button>}
    {routeState.sessionId?<SessionView key={`${routeState.sessionId}:${revision}`} route={routeState} revision={revision}/>:<Discovery revision={revision}/>} 
  </section>
}

function Discovery({revision}:{revision:number}){
  const params=new URLSearchParams(location.search),notice=params.get('notice')
  const [query,setQuery]=React.useState(params.get('q')||''),[appliedQuery,setAppliedQuery]=React.useState(''),[result,setResult]=React.useState<SessionPage|null>(null),[error,setError]=React.useState(''),[loading,setLoading]=React.useState(true)
  const requestSequence=React.useRef(0),searchTimer=React.useRef<number|undefined>(undefined),searchController=React.useRef<AbortController|undefined>(undefined),initialQuery=React.useRef(query)
  const updateLocation=React.useCallback((value:string)=>{const next=new URLSearchParams(location.search);if(value.trim())next.set('q',value.trim());else next.delete('q');history.replaceState(null,'',`${location.pathname}${next.size?`?${next}`:''}`)},[])
  const search=React.useCallback(async(value:string)=>{const normalized=value.trim(),request=++requestSequence.current;searchController.current?.abort();const controller=new AbortController();searchController.current=controller;setLoading(true);setError('');try{const next=await fetchSessions(normalized,revision,undefined,controller.signal);if(request===requestSequence.current){setResult(next);setAppliedQuery(normalized)}}catch(e){if(request===requestSequence.current&&(e as Error).name!=='AbortError'){setError((e as Error).message);setAppliedQuery(normalized)}}finally{if(request===requestSequence.current)setLoading(false)}},[revision])
  const loadMore=async()=>{if(!result?.nextCursor)return;const next=await fetchSessions(appliedQuery,revision,result.nextCursor);setResult({...next,items:[...result.items,...next.items]})}
  React.useEffect(()=>{searchTimer.current=window.setTimeout(()=>{updateLocation(query);void search(query)},query===initialQuery.current?0:150);return()=>{window.clearTimeout(searchTimer.current);searchController.current?.abort()}},[query,search,updateLocation])
  const pending=loading||query.trim()!==appliedQuery
  return <section className="discovery">
    {notice==='current-session-unavailable'&&<div className="warning" role="status"><strong>We couldn't open this session automatically</strong><p>Find it by searching for words from something you asked Codex, or paste its session ID.</p></div>}
    <form className="search-box" role="search" onSubmit={event=>{event.preventDefault();window.clearTimeout(searchTimer.current);updateLocation(query);void search(query)}}><label htmlFor="session-search">Find a Codex session</label><div><input id="session-search" aria-label="Find a Codex session" type="search" autoComplete="off" value={query} onChange={event=>setQuery(event.target.value)} placeholder="What did you ask Codex to do?"/><button disabled={pending}>{pending?'Searching…':'Search'}</button></div><small>Search by words from your requests, or paste an exact session ID.</small></form>
    <div className="discovery-status" role="status" aria-live="polite">{pending?<><LoaderCircle aria-hidden="true"/>Searching your Codex sessions…</>:result?<>{result.coverage.eligible} session{result.coverage.eligible===1?'':'s'} ready to explore</>:null}</div>
    {error&&!pending?<div className="error"><h2>Sessions couldn't load</h2><p>{error}</p></div>:pending||!result?<div className="discovery-skeleton" aria-label="Search results pending" aria-busy="true">{[0,1,2].map(item=><div key={item}><i/><i/><i/></div>)}</div>:<div className="discovery-results" aria-busy="false">{result.items.map(item=>{const duration=sessionDuration(item),total=item.directTokens!=null&&item.descendantTokens!=null?item.directTokens+item.descendantTokens:null;return <a className="discovery-result" key={item.sessionId} href={inspectorPath(`/context/${encodeURIComponent(item.sessionId)}`,{q:appliedQuery||undefined})}><div className="result-main"><h3>{sessionTitle(item)}</h3><p className="result-project">{item.project||<span>Project not available</span>}</p><p className="result-meta">{item.startedAt?<time dateTime={item.startedAt}>{new Date(item.startedAt).toLocaleString(undefined,{month:'short',day:'numeric',year:'numeric',hour:'numeric',minute:'2-digit'})}</time>:'Date not available'}{duration&&<> · {duration}</>} · {item.completedTurns} completed turn{item.completedTurns===1?'':'s'} · {item.descendantSessions} delegated task{item.descendantSessions===1?'':'s'}</p><code>{item.sessionId}</code>{item.matchCategories.map(category=><span className="match-category" key={category}>{matchLabel(category)}</span>)}{item.matchSnippets.slice(0,2).map((snippet,index)=><blockquote className="match-snippet" key={`${snippet.category}-${index}`}><span>{matchLabel(snippet.category)}</span><p><Highlight text={snippet.text} query={appliedQuery}/></p></blockquote>)}</div><dl><dt>All work</dt><dd>{tokens(total)}</dd><dt>Main session</dt><dd>{tokens(item.directTokens)}</dd><dt>Delegated work</dt><dd>{tokens(item.descendantTokens)}</dd></dl><span className="open-result" aria-hidden="true">Explore session <ArrowRight/></span></a>})}{result.nextCursor&&<button className="load-more" onClick={()=>void loadMore()}>Show more sessions</button>}{result.items.length===0&&<div className="empty"><h2>No sessions matched your search</h2><p>Try different words from something you asked Codex, or paste an exact session ID.</p></div>}</div>}
  </section>
}

function SessionView({route,revision}:{route:Route;revision:number}){
  const [map,setMap]=React.useState<SessionMap|null>(null),[summary,setSummary]=React.useState<SessionPage['items'][number]|null>(null),[ledger,setLedger]=React.useState<LedgerPage|null>(null),[error,setError]=React.useState(''),[collapsed,setCollapsed]=React.useState<Set<string>>(new Set()),[turnLimits,setTurnLimits]=React.useState<Map<string,number>>(new Map()),[scale,setScale]=React.useState(1),[pan,setPan]=React.useState({x:0,y:0}),[drag,setDrag]=React.useState<{x:number;y:number;px:number;py:number}|null>(null),[mapExpanded,setMapExpanded]=React.useState(!route.turnId)
  const mapStageRef=React.useRef<HTMLDivElement|null>(null)
  React.useEffect(()=>{let live=true;setError('');setSummary(null);fetchSessionMap(route.sessionId!,revision).then(value=>{if(!live)return;if(value.turns.length>largeMapTurnThreshold)setCollapsed(new Set(value.nodes.filter(node=>node.sessionId!==value.rootSessionId).map(node=>node.sessionId)));setMap(value);void fetchSessionMetadata([value.rootSessionId],revision).then(page=>{if(live)setSummary(page.items[0]||null)}).catch(()=>{/* Stable opaque identity remains available. */})}).catch(e=>{if(live)setError((e as Error).message)});return()=>{live=false}},[route.sessionId,revision])
  React.useEffect(()=>{let live=true;if(!route.turnId||!map){setLedger(null);return}setLedger(null);void (async()=>{try{let page=await fetchLedger(map.rootSessionId,route.turnId!,revision),items=[...page.items],cursor=page.nextCursor;if(!live)return;setLedger({...page,items:[...items],nextCursor:cursor});while(cursor&&items.length<10000){page=await fetchLedger(map.rootSessionId,route.turnId!,revision,cursor);items=[...items,...page.items];cursor=page.nextCursor;if(!live)return;React.startTransition(()=>setLedger({...page,items:[...items],nextCursor:cursor}))}}catch(e){if(live){setLedger(current=>current?{...current,nextCursor:undefined}:null);setError((e as Error).message)}}})();return()=>{live=false}},[map,route.turnId,revision])
  React.useEffect(()=>{const stage=mapStageRef.current;if(!stage)return;const handleWheel=(event:WheelEvent)=>{if(!event.metaKey)return;event.preventDefault();setScale(value=>Math.max(.5,Math.min(1.8,value+(event.deltaY<0?.1:-.1))))};stage.addEventListener('wheel',handleWheel,{passive:false});return()=>stage.removeEventListener('wheel',handleWheel)},[map,mapExpanded])
  if(error&&!map)return <section className="error"><h2>We couldn't open this session</h2><p>{error}</p><button onClick={()=>navigate(inspectorPath('/context'))}>Back to all sessions</button></section>
  if(!map)return <p className="loading">Loading the story of this session…</p>
  type Turn=SessionMap['turns'][number]
  type Relation={parentSessionId:string;childSessionId:string;spawnTurnId:string;edgeKind:string;ordinal:number}
  const root=map.nodes.find(node=>node.kind==='root'),turns=map.turns||[],selectedTurn=turns.find(turn=>turn.turnId===route.turnId)
  const nodeBySession=new Map(map.nodes.map(node=>[node.sessionId,node]))
  const turnsBySession=new Map<string,Turn[]>()
  for(const turn of turns){const list=turnsBySession.get(turn.sessionId)||[];list.push(turn);turnsBySession.set(turn.sessionId,list)}
  for(const list of turnsBySession.values())list.sort((a,b)=>a.ordinal-b.ordinal)
  const relations:Relation[]=map.spawnTopology.map(item=>({...item}))
  for(const node of map.nodes){if(node.sessionId===map.rootSessionId||relations.some(item=>item.childSessionId===node.sessionId))continue;const edge=map.edges.find(item=>item.childSessionId===node.sessionId&&item.kind!=='forked_from');if(edge)relations.push({parentSessionId:edge.parentSessionId,childSessionId:node.sessionId,spawnTurnId:node.spawningTurnId||'',edgeKind:edge.kind,ordinal:relations.length})}
  const maxTurnTokens=Math.max(1,...turns.map(turn=>turn.inclusiveTokens||turn.directTokens||0))
  const largeMap=turns.length>largeMapTurnThreshold
  const relationByChild=new Map(relations.map(relation=>[relation.childSessionId,relation]))
  const sessionVisible=(sessionId:string,seen=new Set<string>()):boolean=>{if(sessionId===map.rootSessionId)return true;if(collapsed.has(sessionId)||seen.has(sessionId))return false;seen.add(sessionId);const parent=relationByChild.get(sessionId)?.parentSessionId;return parent?sessionVisible(parent,seen):true}
  const sessionTurnLimit=(sessionId:string)=>largeMap?(turnLimits.get(sessionId)||turnRenderBatchSize):Number.POSITIVE_INFINITY
  const visibleTurns=largeMap?turns.filter(turn=>sessionVisible(turn.sessionId)&&turn.ordinal<sessionTurnLimit(turn.sessionId)):turns
  const railTurns=largeMap&&selectedTurn?(turnsBySession.get(selectedTurn.sessionId)||[]):turns
  const visibleRailTurns=railTurns.slice(0,largeMap&&selectedTurn?sessionTurnLimit(selectedTurn.sessionId):railTurns.length)
  const turnWidth=(turn:Turn)=>Math.max(132,Math.min(520,132+Math.round((turn.inclusiveTokens||turn.directTokens||0)/maxTurnTokens*388)))
  const turnName=(turn:Turn)=>`${turn.sessionKind==='root'?'Turn':'Agent turn'} ${turn.ordinal+1}`
  const turnPrompt=(turn:Turn)=>turn.promptPreview?.trim()||''
  const turnDisplay=(turn:Turn)=>turnPrompt(turn)?`“${turnPrompt(turn)}”`:turnName(turn)
  const turnAriaLabel=(turn:Turn)=>turnPrompt(turn)?`${turnName(turn)}: ${turnPrompt(turn)}`:turnName(turn)
  const selectTurn=(turn:Turn)=>{setMapExpanded(false);navigate(inspectorPath(`/context/${encodeURIComponent(map.rootSessionId)}/turn/${encodeURIComponent(turn.turnId)}`))}
  const fitSession=()=>{setScale(Math.max(.55,Math.min(1,8/Math.max(8,turns.length))));setPan({x:0,y:0})}
  const fitSelection=()=>{setScale(1.12);setPan({x:0,y:0})}
  const resetZoom=()=>{setScale(1);setPan({x:0,y:0})}
  const TurnCard=({turn}:{turn:Turn})=>{const inclusive=turn.inclusiveTokens||turn.directTokens||0,direct=turn.directTokens||0,directPercent=inclusive?Math.max(6,Math.min(100,Math.round(direct/inclusive*100))):0;return <button type="button" className={`turn-map-node ${turn.sessionKind}${turn.turnId===route.turnId?' selected':''}`} style={{width:turnWidth(turn)}} aria-label={turnAriaLabel(turn)} aria-pressed={turn.turnId===route.turnId} onPointerDown={event=>event.stopPropagation()} onClick={()=>selectTurn(turn)}><span className="turn-direct-fill" style={{width:`${directPercent}%`}} aria-hidden="true"/><span className="turn-node-content"><span><strong title={turnPrompt(turn)||undefined}>{turnDisplay(turn)}</strong><small>{turnPrompt(turn)&&<>{turnName(turn)} · </>}{kindLabel(turn.state)} · {new Date(turn.startedAt).toLocaleTimeString()}</small><span className="turn-token-line">{tokens(turn.directTokens)} in this turn · {tokens(turn.inclusiveTokens)} with delegated work</span></span><span className="turn-badges"><i>{turn.toolCount} tool{turn.toolCount===1?'':'s'}</i>{turn.errorCount>0&&<i className="error-badge">{turn.errorCount} error{turn.errorCount===1?'':'s'}</i>}{turn.compactionCount>0&&<i className="compaction-badge">{turn.compactionCount} context refresh{turn.compactionCount===1?'':'es'}</i>}</span></span></button>}
  const TurnLane=({sessionId}:{sessionId:string})=>{const sessionTurns=turnsBySession.get(sessionId)||[],limit=sessionTurnLimit(sessionId),shown=sessionTurns.slice(0,limit),remaining=sessionTurns.length-shown.length;return <><div className={`turn-map-lane${sessionId===map.rootSessionId?' root-lane':''}`}>{shown.map(turn=><TurnCard turn={turn} key={turn.turnId}/>)}</div>{remaining>0&&<button type="button" className="turn-load-more" onPointerDown={event=>event.stopPropagation()} onClick={()=>setTurnLimits(value=>new Map(value).set(sessionId,limit+turnRenderBatchSize))}>Show next {tokens(Math.min(turnRenderBatchSize,remaining))} turns</button>}</>}
  const renderBranches=(parentSessionId:string,depth=0,path=new Set<string>()):React.ReactNode=>relations.filter(item=>item.parentSessionId===parentSessionId).sort((a,b)=>a.ordinal-b.ordinal).map(relation=>{if(path.has(relation.childSessionId))return null;const spawnTurn=(turnsBySession.get(parentSessionId)||[]).find(turn=>turn.turnId===relation.spawnTurnId),node=nodeBySession.get(relation.childSessionId),branchPrompt=(turnsBySession.get(relation.childSessionId)||[]).find(turn=>turnPrompt(turn))?.promptPreview,nextPath=new Set(path).add(relation.childSessionId),isCollapsed=collapsed.has(relation.childSessionId),offset=depth?28:Math.min(420,32+(spawnTurn?.ordinal||0)*76);return <section className={`agent-turn-branch${isCollapsed?' collapsed':''}`} style={{'--branch-offset':`${offset}px`} as React.CSSProperties} key={`${relation.parentSessionId}:${relation.childSessionId}:${relation.edgeKind}`}><header><span>↳ {spawnTurn?`Delegated from ${turnName(spawnTurn)}`:'Related delegated work'}{branchPrompt&&<> · “{branchPrompt}”</>} · <b>{relation.childSessionId}</b> · {tokens(node?.inclusiveTokens)} tokens</span><button type="button" aria-expanded={!isCollapsed} onPointerDown={event=>event.stopPropagation()} onClick={()=>setCollapsed(value=>{const next=new Set(value);if(next.has(relation.childSessionId))next.delete(relation.childSessionId);else next.add(relation.childSessionId);return next})}>{isCollapsed?'Show delegated work':'Hide delegated work'}</button></header>{!isCollapsed&&<div className="agent-branch-body"><TurnLane sessionId={relation.childSessionId}/>{renderBranches(relation.childSessionId,depth+1,nextPath)}</div>}</section>})
  const title=summary?sessionTitle(summary):map.rootSessionId
  return <section className="session-view"><div className="inspector-breadcrumb"><button onClick={()=>navigate(inspectorPath('/context'))}>All sessions</button><span>/</span><button className="root-crumb" onClick={()=>setMapExpanded(true)}>{title}</button>{route.turnId&&<><span>/</span><span aria-current="page">{selectedTurn?turnName(selectedTurn):route.turnId}</span></>}<a className="quiet-link" href={`/reviews/new?session=${encodeURIComponent(map.rootSessionId)}`}>Learn from this session</a></div>
    <div className="session-identity"><span className="section-label">CODEX SESSION</span><h2>{title}</h2><p>{summary?.project||'Project not available'} · {summary?.startedAt?new Date(summary.startedAt).toLocaleString():'Start time not available'} · {summary?.completedTurns??map.rootTurns.length} completed turns</p></div>
    {map.coverage.fidelity!=='exact'&&<div className="warning"><strong>Some connections may be missing</strong><p>{map.coverage.reason||"Inspector couldn't connect every piece of delegated work to the main session."}</p></div>}
    {mapExpanded&&<>{largeMap&&<div className="large-map-notice" role="status"><strong>Long session · {tokens(turns.length)} turns and {tokens(map.nodes.length)} delegated work streams</strong><span>Delegated work starts folded up so this view stays fast. Open only the parts you want to explore.</span></div>}<div className="map-toolbar"><div><span className="section-label">SESSION OVERVIEW</span><strong>How the work unfolded</strong><small>Each card is one Codex turn. Wider cards used more tokens, and totals include any work Codex delegated.</small></div><div><button onClick={fitSession}>Show whole session</button><button onClick={fitSelection} disabled={!selectedTurn}>Center selected turn</button><button onClick={resetZoom}>Reset view</button><output>{Math.round(scale*100)}%</output></div></div>
    <div className="turn-map-legend" aria-label="Session map legend"><span><i className="root-key"/>Main-session turn</span><span><i className="agent-key"/>Delegated turn</span><span><i className="direct-key"/>This turn inside the total</span><span><i className="compaction-key"/>Context refresh</span><span className="map-interaction-hint">⌘ + scroll to zoom</span></div>
    <div className="map-stage" ref={mapStageRef} onPointerDown={event=>setDrag({x:pan.x,y:pan.y,px:event.clientX,py:event.clientY})} onPointerMove={event=>{if(drag)setPan({x:drag.x+event.clientX-drag.px,y:drag.y+event.clientY-drag.py})}} onPointerUp={()=>setDrag(null)} onPointerLeave={()=>setDrag(null)}>
      <div className="map-canvas turn-map-canvas" style={{transform:`translate(${pan.x}px,${pan.y}px) scale(${scale})`}}><div className="map-root-label"><strong>Main session · {title}</strong><span>{tokens(root?.inclusiveTokens)} total tokens</span></div><TurnLane sessionId={map.rootSessionId}/>{renderBranches(map.rootSessionId)}</div>
      <div className="minimap turn-minimap" aria-label="Session map overview">{visibleTurns.map(turn=><i className={`${turn.sessionKind}${turn.turnId===route.turnId?' selected':''}`} key={turn.turnId} style={{flex:Math.max(1,Math.round(turnWidth(turn)/70))}}/>)}<span style={{transform:`translate(${-pan.x/18}px,${-pan.y/18}px) scale(${1/scale})`}}/></div>
    </div></>}
    {route.turnId&&!mapExpanded&&<div className="topology-rail"><div><span className="section-label">WHERE YOU ARE</span><strong title={selectedTurn?turnPrompt(selectedTurn)||undefined:undefined}>{selectedTurn?turnDisplay(selectedTurn):'Selected turn'}</strong><small>{selectedTurn?`${turnName(selectedTurn)} · ${tokens(selectedTurn.directTokens)} in this turn · ${tokens(selectedTurn.inclusiveTokens)} with delegated work`:'Main-session and delegated turns'}</small><button className="expand-session-map" onClick={()=>setMapExpanded(true)}>View full session map</button></div><nav aria-label="Choose another turn">{visibleRailTurns.map(turn=><button className={`${turn.sessionKind}${turn.turnId===route.turnId?' selected':''}`} style={{flex:Math.max(1,Math.round(turnWidth(turn)/70))}} aria-label={turnAriaLabel(turn)} aria-pressed={turn.turnId===route.turnId} key={turn.turnId} onClick={()=>selectTurn(turn)}><span title={turnPrompt(turn)||undefined}>{turnDisplay(turn)}</span><small>{turnName(turn)} · {tokens(turn.inclusiveTokens)} tokens</small></button>)}{largeMap&&selectedTurn&&visibleRailTurns.length<railTurns.length&&<button onClick={()=>setTurnLimits(value=>new Map(value).set(selectedTurn.sessionId,sessionTurnLimit(selectedTurn.sessionId)+turnRenderBatchSize))}>Show more turns</button>}</nav></div>}
    {route.turnId?<TurnInspector rootId={map.rootSessionId} turnId={route.turnId} eventId={route.eventId} evidenceId={route.evidenceId} revision={revision} ledger={ledger}/>:<div className="focus-empty"><h2>Choose a turn to see what happened</h2><p>Select any card to explore the messages, tools, token usage, and delegated work from that part of the session.</p><p><span className="derived-badge">Token summary</span> {tokens(root?.directTokens)} in the main session · {tokens(root?.inclusiveTokens)} including delegated work. These are totals; Inspector does not guess how tokens split within a turn.</p></div>}
  </section>
}

function TurnInspector({rootId,turnId,eventId,evidenceId,revision,ledger}:{rootId:string;turnId:string;eventId?:string;evidenceId?:string;revision:number;ledger:LedgerPage|null}){
  const selected=ledger?.items.find(item=>item.eventId===eventId||item.evidenceId===evidenceId)
  const activeEvidence=evidenceId||selected?.evidenceId
  const [chunks,setChunks]=React.useState<EvidenceChunk[]>([]),[error,setError]=React.useState(''),[view,setView]=React.useState<'story'|'raw'>('story'),[filter,setFilter]=React.useState<LedgerFilter>('all'),[query,setQuery]=React.useState(''),[expanded,setExpanded]=React.useState<Set<string>>(new Set())
  const eventRef=React.useRef<HTMLElement|null>(null)
  const searchRef=React.useRef<HTMLInputElement|null>(null),groupButtonRefs=React.useRef(new Map<string,HTMLButtonElement>())
  const storyGroups=React.useMemo(()=>makeLedgerGroups(ledger?.items||[]),[ledger]),rawGroups=React.useMemo(()=>rawLedgerGroups(ledger?.items||[]),[ledger]),groups=view==='story'?storyGroups:rawGroups
  const normalizedQuery=query.trim().toLowerCase()
  const visibleGroups=groups.filter(group=>{
    const containsSelection=group.items.some(item=>item.eventId===selected?.eventId)
    if(!containsSelection&&!groupMatchesFilter(group,filter))return false
    if(!normalizedQuery||containsSelection)return true
    return [group.label,group.detail,...group.items.flatMap(item=>[item.kind,item.messageRole||'',item.toolName||'',item.status||''])].some(value=>value.toLowerCase().includes(normalizedQuery))
  })
  React.useEffect(()=>{eventRef.current?.scrollIntoView?.({block:'nearest',behavior:'smooth'})},[selected?.eventId,ledger])
  React.useEffect(()=>{const selectedGroup=storyGroups.find(group=>group.items.some(item=>item.eventId===selected?.eventId));if(selectedGroup&&selectedGroup.items.length>1)setExpanded(value=>{if(value.has(selectedGroup.id))return value;const next=new Set(value);next.add(selectedGroup.id);return next})},[selected?.eventId,storyGroups])
  React.useEffect(()=>{let live=true;setChunks([]);setError('');if(!activeEvidence)return;fetchEvidence(activeEvidence,revision).then(chunk=>{if(live)setChunks([chunk])}).catch(e=>{if(live)setError((e as Error).message)});return()=>{live=false}},[activeEvidence,revision])
  if(!ledger)return <p className="loading">Loading what happened in this turn…</p>
  const current=chunks.at(-1),raw=chunks.map(chunk=>chunk.text||'').join('')
  const selectItem=(item:LedgerItem)=>navigate(inspectorPath(`/context/${encodeURIComponent(rootId)}/turn/${encodeURIComponent(turnId)}/event/${encodeURIComponent(item.eventId)}`,{evidence:item.evidenceId}))
  const focusGroup=(group:LedgerGroup)=>window.setTimeout(()=>groupButtonRefs.current.get(group.id)?.focus(),0)
  const jumpTo=(predicate:(group:LedgerGroup)=>boolean)=>{const target=storyGroups.find(predicate);if(!target)return;setView('story');setFilter('all');setQuery('');focusGroup(target)}
  const moveFocus=(direction:1|-1,predicate:((group:LedgerGroup)=>boolean)=()=>true)=>{const candidates=visibleGroups.filter(predicate);if(!candidates.length)return;const focused=[...groupButtonRefs.current.entries()].find(([,button])=>button===document.activeElement)?.[0],currentIndex=Math.max(-1,candidates.findIndex(group=>group.id===focused||group.items.some(item=>item.eventId===selected?.eventId))),next=currentIndex<0?(direction>0?0:candidates.length-1):(currentIndex+direction+candidates.length)%candidates.length;groupButtonRefs.current.get(candidates[next].id)?.focus()}
  const handleLedgerKey=(event:React.KeyboardEvent<HTMLElement>)=>{const target=event.target as HTMLElement,isField=target.matches('input,textarea,select');if(isField){if(event.key==='Escape'){setQuery('');searchRef.current?.blur()}return}if(event.key==='/'){event.preventDefault();searchRef.current?.focus();return}if(event.key.toLowerCase()==='j'||event.key.toLowerCase()==='k'){event.preventDefault();moveFocus(event.key.toLowerCase()==='j'?1:-1);return}if(event.key.toLowerCase()==='t'){event.preventDefault();moveFocus(event.shiftKey?-1:1,group=>group.family==='tool');return}if(event.key.toLowerCase()==='e'){event.preventDefault();moveFocus(1,group=>group.hasError)}}
  const toolCount=storyGroups.filter(group=>group.family==='tool').length,runtimeCount=storyGroups.filter(group=>group.actor==='runtime').reduce((sum,group)=>sum+group.items.length,0),requestCount=storyGroups.filter(group=>group.actor==='user').length,responseCount=storyGroups.filter(group=>group.actor==='agent'&&group.family==='message').length,compactionCount=storyGroups.filter(group=>group.family==='compaction').length
  return <div className="turn-focus"><section className="ledger" aria-label="What happened in this turn" aria-busy={Boolean(ledger.nextCursor)} onKeyDown={handleLedgerKey}><div className="ledger-heading"><span className="section-label">TURN DETAILS</span><h2>What happened in this turn</h2><p>{ledger.items.length} saved moments grouped into {storyGroups.length} steps. You can switch to the original event list at any time.</p>{ledger.nextCursor?<div className="ledger-loading-more" role="status" aria-live="polite"><LoaderCircle aria-hidden="true"/><span>{ledger.items.length} moments ready · loading more…</span></div>:null}</div>
    <nav className="turn-landmarks" aria-label="Jump to part of this turn"><button onClick={()=>jumpTo(group=>group.actor==='user')} disabled={!requestCount}><UserRound aria-hidden="true"/><span>Your request</span><strong>{requestCount}</strong></button><button onClick={()=>jumpTo(group=>group.actor==='runtime')} disabled={!runtimeCount}><ServerCog aria-hidden="true"/><span>Setup</span><strong>{runtimeCount}</strong></button><button onClick={()=>jumpTo(group=>group.family==='tool')} disabled={!toolCount}><Wrench aria-hidden="true"/><span>Tools</span><strong>{toolCount}</strong></button><button onClick={()=>jumpTo(group=>group.family==='compaction')} disabled={!compactionCount}><AlertTriangle aria-hidden="true"/><span>Context updates</span><strong>{compactionCount}</strong></button><button onClick={()=>jumpTo(group=>group.actor==='agent'&&group.family==='message')} disabled={!responseCount}><Bot aria-hidden="true"/><span>Codex replies</span><strong>{responseCount}</strong></button></nav>
    <div className="ledger-controls"><div className="ledger-view-toggle" role="group" aria-label="Choose how much detail to show"><button aria-pressed={view==='story'} onClick={()=>setView('story')}>Overview</button><button aria-pressed={view==='raw'} onClick={()=>setView('raw')}>Original events</button></div><label className="ledger-search"><Search aria-hidden="true"/><span>Search this turn</span><input ref={searchRef} type="search" value={query} onChange={event=>setQuery(event.target.value)} placeholder="Message, tool, or action…"/></label></div>
    <div className="ledger-filters" role="group" aria-label="Filter this turn">{([['all','All'],['messages','Messages'],['tools','Tools'],['runtime','Setup'],['errors','Errors']] as Array<[LedgerFilter,string]>).map(([value,label])=><button key={value} aria-pressed={filter===value} onClick={()=>setFilter(value)}>{label}</button>)}</div>
    <p className="ledger-shortcuts">Keyboard: <kbd>J</kbd>/<kbd>K</kbd> move between steps · <kbd>T</kbd> next tool · <kbd>⇧T</kbd> previous tool · <kbd>E</kbd> next error · <kbd>/</kbd> search</p>
    {visibleGroups.length?<ol className={`ledger-groups ${view}`} aria-label={view==='story'?'Turn overview':'Original events in time order'}>{visibleGroups.map(group=>{const primary=group.items.find(item=>item.toolPhase==='request')||group.items[0],ordinal=ledger.items.findIndex(item=>item.eventId===primary.eventId)+1,active=group.items.some(item=>item.eventId===selected?.eventId),isExpanded=expanded.has(group.id),expandable=view==='story'&&group.items.length>1;return <li key={group.id}><article ref={active?eventRef:undefined} className={`${active?'selected ':''}actor-${group.actor}${group.hasError?' has-error':''}`}><div className="ledger-group-row"><button className="ledger-group-main" ref={node=>{if(node)groupButtonRefs.current.set(group.id,node);else groupButtonRefs.current.delete(group.id)}} aria-current={active?'true':undefined} aria-label={`${String(ordinal).padStart(2,'0')} ${eventLabel(primary.kind)} · ${group.label}`} onClick={()=>selectItem(primary)}><span className="ledger-ordinal">{String(ordinal).padStart(2,'0')}</span><span className={`actor-mark ${group.actor}`}><LedgerActorIcon actor={group.actor}/></span><span className="ledger-group-copy"><strong>{group.label}</strong><small><b>{actorLabels[group.actor]}</b> · {group.detail}</small></span><time>{new Date(primary.observedAt).toLocaleTimeString()}</time></button>{expandable?<button className="ledger-expand" aria-label={`${isExpanded?'Hide':'Show'} ${group.label} details`} aria-expanded={isExpanded} onClick={()=>setExpanded(value=>{const next=new Set(value);if(next.has(group.id))next.delete(group.id);else next.add(group.id);return next})}>{isExpanded?<ChevronDown aria-hidden="true"/>:<ChevronRight aria-hidden="true"/>}</button>:null}</div>
        {isExpanded&&expandable?<div className="ledger-group-events">{group.items.map(item=>{const itemOrdinal=ledger.items.findIndex(candidate=>candidate.eventId===item.eventId)+1,itemActive=item.eventId===selected?.eventId,phase=item.toolPhase?item.toolPhase==='request'?'Request':'Result':eventLabel(item.kind);return <button key={item.eventId} aria-label={`${String(itemOrdinal).padStart(2,'0')} ${phase} ${eventLabel(item.kind)}`} aria-current={itemActive?'true':undefined} onClick={()=>selectItem(item)}><span>{String(itemOrdinal).padStart(2,'0')}</span><strong>{phase}</strong><small>{eventLabel(item.kind)}</small><time>{new Date(item.observedAt).toLocaleTimeString()}</time></button>})}</div>:null}
        {group.family==='compaction'?<small className="ledger-event-note">Codex refreshed its working context here. Token totals before and after are still shown.</small>:null}</article></li>})}</ol>:<div className="ledger-empty"><MessageSquareText aria-hidden="true"/><strong>Nothing matches</strong><span>Try a different filter or clear your search.</span></div>}</section>
    <section className="evidence-panel"><span className="section-label">SELECTED MOMENT</span>{!activeEvidence?<><h2>Choose a step</h2><p>Select anything from the turn to see its saved details here.</p></>:error?<div className="error"><h2>Details unavailable</h2><p>{error}</p></div>:!current?<p className="loading">Loading the original record…</p>:<><div className="evidence-heading"><div><h2>{selected?eventLabel(selected.kind):'Linked session moment'}</h2><p>Saved from this session · data snapshot {revision}</p></div><span className={current.availability==='available'?'exact-badge':'unavailable-badge'}>{current.availability==='available'?'Original record':'Unavailable'}</span></div>
      {current.availability!=='available'?<div className="unavailable-context"><strong>Original details unavailable</strong><p>The local record is {current.availability.replaceAll('_',' ')}. You can still explore the session timeline and token totals.</p></div>:<><EvidenceView kind={selected?.kind||'record'} observedAt={selected?.observedAt||current.availabilityObservedAt} raw={raw} complete={current.complete}/><details className="original-record"><summary>View original Codex record</summary><div className="local-evidence-notice"><strong>This is the original local record</strong><p>It may include secrets, messages, tool inputs, or results. Inspector shows it safely as text and does not send it anywhere.</p></div><pre className="raw-evidence" aria-label="Original Codex record">{current.complete&&parseJSON(raw)!==undefined?displayJSON(parseJSON(raw)):raw}</pre><p className="chunk-meta">{current.encoding==='escaped-bytes'?'Control characters are shown safely as text; nothing runs.':'Shown safely as text.'} {chunks.reduce((sum,chunk)=>sum+chunk.bytes,0)} bytes shown.</p>{!current.complete&&<button onClick={()=>void fetchEvidence(activeEvidence,revision,current.offset+current.bytes).then(chunk=>setChunks(value=>[...value,chunk]))}>Load more of this record</button>}</details></>}</>}
    </section>
  </div>
}
