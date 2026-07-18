import {afterEach,describe,expect,it,vi} from 'vitest'
import {cleanup,fireEvent,render,screen} from '@testing-library/react'
import '@testing-library/jest-dom/vitest'
import {App} from './app'
import {establishSession} from './api'

afterEach(()=>{cleanup();vi.restoreAllMocks();history.replaceState(null,'','/')})

const status = (overrides: Record<string, unknown> = {}) => ({
  process:{state:'ready',inspectorVersion:'0.1.0',cliVersion:'0.1.0',cliCompatibility:'supported',pluginVersion:'0.1.0',pluginProtocolVersion:1,pid:1,startedAt:'2026-07-18T00:00:00Z', ...overrides},
  index:{state:'empty',schemaVersion:1,databaseBytes:0,sourceCount:0,supportedSourceCount:0,unsupportedSourceCount:0,pendingTailCount:0,queuedSessionChanges:0,processedCount:0,queuedCount:0,skippedCount:0,failedCount:0,requiresRebuildCount:0,reverseScanBoundary:null,completedWatermark:null},
  hook:{state:'idle',registeredEvents:['SessionStart','UserPromptSubmit','PreCompact','PostCompact','SubagentStart','SubagentStop','Stop'],lastMarker:null,diagnostics:[]},
	coverage:{fidelity:'unavailable',observed:0,eligible:0,reason:'indexing begins in Phase 2'},
})

describe('Inspector shell',()=>{
  it('sends the exact frozen bootstrap and removes fragment after exchange',async()=>{
    history.replaceState(null,'','/#token=fake-token-abcdefghijklmnopqrstuvwxyz&instanceId=instance-1&protocolVersion=1')
    const fetchMock=vi.spyOn(globalThis,'fetch').mockResolvedValueOnce(new Response(null,{status:204})).mockResolvedValueOnce(new Response(JSON.stringify(status()),{status:200}))
    await establishSession()
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(JSON.parse(String(fetchMock.mock.calls[0][1]?.body))).toEqual({token:'fake-token-abcdefghijklmnopqrstuvwxyz',instanceId:'instance-1',protocolVersion:1})
    expect(location.hash).toBe('')
  })
  it('rejects incomplete startup metadata without a request',async()=>{
    history.replaceState(null,'','/#token=fake-token-abcdefghijklmnopqrstuvwxyz')
    const fetchMock=vi.spyOn(globalThis,'fetch')
    await expect(establishSession()).rejects.toThrow('startup metadata')
    expect(fetchMock).not.toHaveBeenCalled()
  })
  it('shows an honest ready state and complete tray',async()=>{
    vi.spyOn(globalThis,'fetch').mockResolvedValue(new Response(JSON.stringify(status()),{status:200}))
    const writeText=vi.fn().mockResolvedValue(undefined);Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText}})
    render(<App/>);expect(await screen.findByText('Ready for your first sync')).toBeInTheDocument();expect(screen.getByText('Status & debugging')).toBeInTheDocument();for(const label of ['Inspector / CLI / process','Codex host compatibility','Source format / data home','Hook trust / server','Source inventory','Reverse scan / watermark','Database / schema','Registered hooks','Last marker','Hook diagnostics'])expect(screen.getByText(label)).toBeInTheDocument();fireEvent.click(screen.getByText('Copy doctor diagnostics'));expect(writeText).toHaveBeenCalledTimes(1)
    const copied=JSON.parse(writeText.mock.calls[0][0]);expect(copied.checks.map((c:{name:string})=>c.name)).toEqual(['inspector_cli','codex_host','source_format','data_home','plugin','hook_trust','server']);expect(copied.tray.sources).toEqual(expect.objectContaining({total:0,supported:0,unsupported:0,pendingTail:0}));expect(copied.tray.database).toEqual({bytes:0,schemaVersion:1});expect(copied.tray.hooks.registeredEvents).toHaveLength(7);expect(JSON.stringify(copied)).not.toContain('accessToken')
  })
  it('renders actual setup and incompatible states',async()=>{
    vi.spyOn(globalThis,'fetch').mockResolvedValueOnce(new Response(JSON.stringify(status({cliCompatibility:'unknown',pluginVersion:null,pluginProtocolVersion:null,state:'degraded'})),{status:200}))
    render(<App/>);expect(await screen.findByText('Finish Inspector setup')).toBeInTheDocument();cleanup()
    vi.spyOn(globalThis,'fetch').mockResolvedValueOnce(new Response(JSON.stringify(status({cliCompatibility:'unsupported',state:'degraded'})),{status:200}))
    render(<App/>);expect(await screen.findByText('Inspector versions are incompatible')).toBeInTheDocument()
  })
  it('renders a recoverable error state',async()=>{
    vi.spyOn(globalThis,'fetch').mockResolvedValue(new Response(null,{status:401}));render(<App/>);expect(await screen.findByText('Unable to open Inspector')).toBeInTheDocument();expect(screen.getByText('codex-inspector doctor')).toBeInTheDocument()
  })
})
