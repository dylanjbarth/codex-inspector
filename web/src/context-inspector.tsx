import React from 'react'
import {fetchEvidence,fetchLedger,fetchRecordedContext,fetchSessionMap,fetchSessions,type EvidenceChunk,type LedgerPage,type RecordedContext,type SessionMap,type SessionPage} from './api'

type Props={revision:number;available:number|null;onApply:()=>Promise<void>}
type Route={sessionId?:string;turnId?:string;eventId?:string;evidenceId?:string}

function route():Route{
  const parts=location.pathname.split('/').filter(Boolean).map(value=>{try{return decodeURIComponent(value)}catch{return ''}})
  if(parts[0]!=='context')return {}
  const eventIndex=parts.indexOf('event'),turnIndex=parts.indexOf('turn')
  return {sessionId:parts[1],turnId:turnIndex>=0?parts[turnIndex+1]:undefined,eventId:eventIndex>=0?parts[eventIndex+1]:undefined,evidenceId:new URLSearchParams(location.search).get('evidence')||undefined}
}
function navigate(path:string){history.pushState(null,'',path);dispatchEvent(new PopStateEvent('popstate'))}
const tokens=(value:number|null|undefined)=>value==null?'Unavailable':new Intl.NumberFormat().format(value)
const kindLabel=(value:string)=>value.replaceAll('_',' ').replace(/\b\w/g,letter=>letter.toUpperCase())

export function ContextInspector({revision,available,onApply}:Props){
  const [routeState,setRoute]=React.useState(route())
  const returnTo=new URLSearchParams(location.search).get('return')
  React.useEffect(()=>{const update=()=>setRoute(route());addEventListener('popstate',update);return()=>removeEventListener('popstate',update)},[])
  return <section className="inspector-shell"><header><div><span className="eyebrow">SOURCE-BACKED TRACE</span><h1>Context Inspector</h1></div><nav>{returnTo&&returnTo.startsWith('/reviews/')&&<a className="quiet-link" href={returnTo}>Return to Review</a>}</nav></header>
    {available!=null&&available!==revision&&<button className="new-data" onClick={()=>void onApply()}>New data available — preserve this view &amp; apply</button>}
    <div className="fidelity-legend" aria-label="Evidence fidelity"><span><i className="exact-dot"/>Exact source evidence</span><span><i className="derived-dot"/>Derived aggregate</span><span><i className="unavailable-dot"/>Unavailable in demo</span></div>
    {routeState.sessionId?<SessionView key={`${routeState.sessionId}:${revision}`} route={routeState} revision={revision}/>:<Discovery revision={revision}/>} 
    <aside className="sensitive-warning">Sensitive local data: exact evidence can contain secrets, messages, tool arguments, or results. Inspector renders it as inert text and never adds masking.</aside>
  </section>
}

function Discovery({revision}:{revision:number}){
  const params=new URLSearchParams(location.search),notice=params.get('notice')
  const [query,setQuery]=React.useState(params.get('q')||''),[result,setResult]=React.useState<SessionPage|null>(null),[error,setError]=React.useState(''),[loading,setLoading]=React.useState(true)
  const initialQuery=React.useRef(query)
  const search=React.useCallback(async(value:string)=>{setLoading(true);setError('');try{setResult(await fetchSessions(value,revision))}catch(e){setError((e as Error).message)}finally{setLoading(false)}},[revision])
  const loadMore=async()=>{if(!result?.nextCursor)return;const next=await fetchSessions(query,revision,result.nextCursor);setResult({...next,items:[...result.items,...next.items]})}
  React.useEffect(()=>{void search(initialQuery.current)},[search])
  return <section className="discovery"><div className="discovery-intro"><p className="section-label">SESSION DISCOVERY</p><h2>Find a recorded thread of work</h2><p>Search titles, projects, working directories, IDs, readable messages, and readable tool results. Descendant matches stay nested under their root.</p></div>
    {notice==='current-session-unavailable'&&<div className="warning" role="status"><strong>Unable to locate the current session ID</strong><p>Inspector did not guess from the working directory or timestamps. Search the indexed session history instead.</p></div>}
    <form className="search-box" onSubmit={event=>{event.preventDefault();const next=new URLSearchParams(location.search);if(query)next.set('q',query);else next.delete('q');history.replaceState(null,'',`${location.pathname}${next.size?`?${next}`:''}`);void search(query)}}><label htmlFor="session-search">Search recorded sessions</label><div><input id="session-search" value={query} onChange={event=>setQuery(event.target.value)} placeholder="Message, tool result, project, title, or ID"/><button>Search</button></div></form>
    {loading?<p className="loading">Reading the applied snapshot…</p>:error?<div className="error"><h2>Discovery unavailable</h2><p>{error}</p></div>:<div className="discovery-results"><p>{result?.items.length||0} root sessions at revision {result?.appliedRevision}</p>{result?.items.map(item=><article key={item.sessionId}><div><h3>{item.title||'Untitled recorded session'}</h3><p>{item.matchCategories.join(' · ')}</p></div><dl><dt>Direct</dt><dd>{tokens(item.directTokens)}</dd><dt>Descendant</dt><dd>{tokens(item.descendantTokens)}</dd></dl><button onClick={()=>navigate(`/context/${encodeURIComponent(item.sessionId)}`)}>Open causal map</button></article>)}{result?.nextCursor&&<button onClick={()=>void loadMore()}>Load more sessions</button>}{result?.items.length===0&&<div className="empty"><h2>No matching completed sessions</h2><p>Try a broader source-backed term. Active and provisional turns are intentionally absent.</p></div>}</div>}
  </section>
}

function SessionView({route,revision}:{route:Route;revision:number}){
  const [map,setMap]=React.useState<SessionMap|null>(null),[ledger,setLedger]=React.useState<LedgerPage|null>(null),[error,setError]=React.useState(''),[collapsed,setCollapsed]=React.useState<Set<string>>(new Set()),[scale,setScale]=React.useState(1),[pan,setPan]=React.useState({x:0,y:0}),[drag,setDrag]=React.useState<{x:number;y:number;px:number;py:number}|null>(null)
  React.useEffect(()=>{let live=true;setError('');fetchSessionMap(route.sessionId!,revision).then(value=>{if(live)setMap(value)}).catch(e=>{if(live)setError((e as Error).message)});return()=>{live=false}},[route.sessionId,revision])
  React.useEffect(()=>{let live=true;if(!route.turnId||!map){setLedger(null);return}void (async()=>{try{let page=await fetchLedger(map.rootSessionId,route.turnId!,revision);const items=[...page.items];let cursor=page.nextCursor;while(cursor&&items.length<10000){page=await fetchLedger(map.rootSessionId,route.turnId!,revision,cursor);items.push(...page.items);cursor=page.nextCursor}if(live)setLedger({...page,items,nextCursor:cursor})}catch(e){if(live){setLedger(null);setError((e as Error).message)}}})();return()=>{live=false}},[map,route.turnId,revision])
  if(error&&!map)return <section className="error"><h2>Session unavailable at this snapshot</h2><p>{error}</p><button onClick={()=>navigate('/context')}>Return to discovery</button></section>
  if(!map)return <p className="loading">Building the causal view from normalized facts…</p>
  const root=map.nodes.find(node=>node.kind==='root'),hidden=new Set<string>()
  const hideChildren=(parent:string)=>{for(const edge of map.edges.filter(edge=>edge.parentSessionId===parent&&edge.kind!=='forked_from')){hidden.add(edge.childSessionId);hideChildren(edge.childSessionId)}}
  for(const id of collapsed)hideChildren(id)
  const visible=map.nodes.filter(node=>!hidden.has(node.sessionId)),maxTokens=Math.max(1,...map.nodes.map(node=>node.inclusiveTokens||0))
  const fit=()=>{setScale(1);setPan({x:0,y:0})}
  return <section className="session-view"><div className="inspector-breadcrumb"><button onClick={()=>navigate('/context')}>Discovery</button><span>/</span><strong>{map.rootSessionId}</strong>{route.turnId&&<><span>/</span><span>{route.turnId}</span></>}<a className="quiet-link" href={`/reviews/new?session=${encodeURIComponent(map.rootSessionId)}`}>Review effectiveness</a></div>
    {map.coverage.fidelity!=='exact'&&<div className="warning"><strong>Partial lineage</strong><p>{map.coverage.reason||'Some causal relationships remain unresolved.'}</p></div>}
    <div className="map-toolbar"><div><strong>Causal session map</strong><small>Absolute node scale · direct and inclusive recorded tokens</small></div><div><button aria-label="Zoom out" onClick={()=>setScale(value=>Math.max(.5,value-.1))}>−</button><output>{Math.round(scale*100)}%</output><button aria-label="Zoom in" onClick={()=>setScale(value=>Math.min(1.8,value+.1))}>+</button><button onClick={fit}>Fit</button></div></div>
    <div className="map-stage" onWheel={event=>{event.preventDefault();setScale(value=>Math.max(.5,Math.min(1.8,value+(event.deltaY<0?.1:-.1))))}} onPointerDown={event=>setDrag({x:pan.x,y:pan.y,px:event.clientX,py:event.clientY})} onPointerMove={event=>{if(drag)setPan({x:drag.x+event.clientX-drag.px,y:drag.y+event.clientY-drag.py})}} onPointerUp={()=>setDrag(null)} onPointerLeave={()=>setDrag(null)}>
      <div className="map-canvas" style={{transform:`translate(${pan.x}px,${pan.y}px) scale(${scale})`}}><svg className="map-edges" viewBox="0 0 1100 600" aria-label="Visible lineage edges">{map.edges.map((edge,index)=>{const from=visible.findIndex(node=>node.sessionId===edge.parentSessionId),to=visible.findIndex(node=>node.sessionId===edge.childSessionId);if(from<0||to<0)return null;const x1=145+(from%4)*240,y1=103+Math.floor(from/4)*160,x2=145+(to%4)*240,y2=103+Math.floor(to/4)*160;return <g key={`${edge.parentSessionId}-${edge.childSessionId}-${edge.kind}-${index}`}><line x1={x1} y1={y1} x2={x2} y2={y2}/><text x={(x1+x2)/2} y={(y1+y2)/2-6}>{edge.kind.replaceAll('_',' ')}</text></g>})}</svg>{visible.map((node,index)=>{const hasChildren=map.edges.some(edge=>edge.parentSessionId===node.sessionId&&edge.kind!=='forked_from');return <article className={`map-node ${node.kind}`} style={{left:40+(index%4)*240,top:45+Math.floor(index/4)*160,width:150+Math.round((node.inclusiveTokens||0)/maxTokens*70)}} key={node.sessionId} onPointerDown={event=>event.stopPropagation()}><span>{node.kind}</span><strong>{node.sessionId}</strong><dl><dt>Direct</dt><dd>{tokens(node.directTokens)}</dd><dt>Inclusive</dt><dd>{tokens(node.inclusiveTokens)}</dd></dl>{node.spawningTurnId&&<small>Spawned at {node.spawningTurnId}</small>}{hasChildren&&<button onClick={()=>setCollapsed(value=>{const next=new Set(value);if(next.has(node.sessionId))next.delete(node.sessionId);else next.add(node.sessionId);return next})}>{collapsed.has(node.sessionId)?'Expand branch':'Collapse branch'}</button>}</article>})}</div>
      <div className="minimap" aria-label="Causal map minimap">{map.nodes.map((node,index)=><i className={hidden.has(node.sessionId)?'hidden':''} key={node.sessionId} style={{left:8+(index%4)*24,top:9+Math.floor(index/4)*18}}/>)}<span style={{transform:`translate(${-pan.x/16}px,${-pan.y/16}px) scale(${1/scale})`}}/></div>
    </div>
    <div className="topology-rail"><div><span className="section-label">ROOT TURN RAIL</span><small>Persistent topology · completed and terminal turns only</small></div>{map.rootTurns.map(turn=><button className={turn.turnId===route.turnId?'selected':''} key={turn.turnId} onClick={()=>navigate(`/context/${encodeURIComponent(map.rootSessionId)}/turn/${encodeURIComponent(turn.turnId)}`)}><span>Turn {turn.ordinal+1}</span><small>{kindLabel(turn.state)} · {new Date(turn.startedAt).toLocaleTimeString()}</small></button>)}</div>
    {route.turnId?<TurnInspector rootId={map.rootSessionId} turnId={route.turnId} eventId={route.eventId} evidenceId={route.evidenceId} revision={revision} ledger={ledger}/>:<div className="focus-empty"><h2>Select a recorded turn</h2><p>The full causal topology stays visible while its chronological source evidence opens below.</p><p><span className="derived-badge">Derived aggregate</span> Root {tokens(root?.directTokens)} direct · {tokens(root?.inclusiveTokens)} inclusive tokens. No per-component estimates.</p></div>}
  </section>
}

function TurnInspector({rootId,turnId,eventId,evidenceId,revision,ledger}:{rootId:string;turnId:string;eventId?:string;evidenceId?:string;revision:number;ledger:LedgerPage|null}){
  const selected=ledger?.items.find(item=>item.eventId===eventId||item.evidenceId===evidenceId)
  const activeEvidence=evidenceId||selected?.evidenceId
  const [chunks,setChunks]=React.useState<EvidenceChunk[]>([]),[context,setContext]=React.useState<RecordedContext|null>(null),[error,setError]=React.useState('')
  const eventRef=React.useRef<HTMLElement|null>(null)
  React.useEffect(()=>{eventRef.current?.scrollIntoView?.({block:'nearest',behavior:'smooth'})},[eventId,ledger])
  React.useEffect(()=>{let live=true;setChunks([]);setContext(null);setError('');if(!activeEvidence)return;Promise.all([fetchEvidence(activeEvidence,revision),fetchRecordedContext(activeEvidence,revision)]).then(([chunk,recorded])=>{if(live){setChunks([chunk]);setContext(recorded)}}).catch(e=>{if(live)setError((e as Error).message)});return()=>{live=false}},[activeEvidence,revision])
  if(!ledger)return <p className="loading">Loading the completed-turn ledger…</p>
  const current=chunks.at(-1),raw=chunks.map(chunk=>chunk.text||'').join('')
  const returnTo=new URLSearchParams(location.search).get('return'),query=(evidence:string)=>{const p=new URLSearchParams({evidence});if(returnTo&&returnTo.startsWith('/reviews/'))p.set('return',returnTo);return p.toString()}
  return <div className="turn-focus"><section className="ledger"><div><span className="section-label">CHRONOLOGICAL EVENT LEDGER</span><h2>Full recorded turn</h2><p>{ledger.items.length} source-backed events. Select one to inspect its exact record.</p></div><ol>{ledger.items.map((item,index)=><li key={item.eventId}><article ref={item.eventId===eventId?eventRef:undefined} className={item.eventId===eventId?'selected':''}><button onClick={()=>navigate(`/context/${encodeURIComponent(rootId)}/turn/${encodeURIComponent(turnId)}/event/${encodeURIComponent(item.eventId)}?${query(item.evidenceId)}`)}><span>{String(index+1).padStart(2,'0')}</span><strong>{kindLabel(item.kind)}</strong><time>{new Date(item.observedAt).toLocaleTimeString()}</time></button>{(item.kind==='compacted'||item.kind==='context_compacted')&&<small>First-class exact recorded compaction event; surrounding ledger usage remains visible.</small>}</article></li>)}</ol></section>
    <section className="evidence-panel"><span className="section-label">EVENT EVIDENCE</span>{!activeEvidence?<><h2>Choose an event</h2><p>Its opaque evidence reference will resolve against the original source file.</p></>:error?<div className="error"><h2>Evidence unavailable</h2><p>{error}</p></div>:!current?<p className="loading">Verifying pinned source fingerprints…</p>:<><div className="evidence-heading"><div><h2>{selected?kindLabel(selected.kind):'Deep-linked event'}</h2><p>Opaque evidence ID · revision {revision}</p></div><span className={current.availability==='available'?'exact-badge':'unavailable-badge'}>{current.availability==='available'?'Exact source':'Unavailable'}</span></div>
      {current.availability!=='available'?<div className="unavailable-context"><strong>Exact payload unavailable</strong><p>The source is {current.availability.replaceAll('_',' ')}. Indexed metrics and normalized facts remain visible.</p></div>:<><pre className="raw-evidence" aria-label="Exact inert source payload">{raw}</pre><p className="chunk-meta">{current.encoding==='escaped-bytes'?'Unsafe/control bytes are visibly escaped; nothing is executed.':'UTF-8 source bytes rendered as inert text.'} {chunks.reduce((sum,chunk)=>sum+chunk.bytes,0)} bytes shown.</p>{!current.complete&&<button onClick={()=>void fetchEvidence(activeEvidence,revision,current.offset+current.bytes).then(chunk=>setChunks(value=>[...value,chunk]))}>Load next bounded chunk</button>}</>}
      <div className={context?.fidelity==='exact'?'exact-context':'unavailable-context'}><strong>{context?.blocks[0]?.kind==='compaction'?'Exact recorded compaction evidence':context?.fidelity==='exact'?'Context seen by Codex':'Unavailable in demo'}</strong><p>{context?.reason||'The supported source proves this exact recorded model-input boundary.'}</p>{context?.blocks[0]?.kind==='compaction'&&<small>No reconstructed before/after context, preserved/removed classification, or component token estimate is claimed.</small>}</div></>}
    </section>
  </div>
}
