import * as React from 'react'
import { BarChart3, BookOpenText, ChevronRight, Menu, PanelLeftClose, PanelLeftOpen, RefreshCw } from 'lucide-react'
import type { Status } from './api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { Sheet, SheetContent, SheetTrigger } from '@/components/ui/sheet'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { handleInternalLinkClick } from './navigation'

type Props = { status: Status; children: React.ReactNode }

const routes = [
  { href: '/', label: 'Dashboard', icon: BarChart3 },
  { href: '/context', label: 'Context Inspector', icon: BookOpenText },
  { href: '/reviews', label: 'Reviews', icon: RefreshCw },
]

function active(href: string) {
  return href === '/' ? location.pathname === '/' : location.pathname.startsWith(href)
}

function Nav({ compact = false }: { compact?: boolean }) {
  return <nav className="shell-nav" aria-label="Inspector sections">
    {routes.map(({ href, label, icon: Icon }) => <a key={href} className={active(href) ? 'shell-nav-link active' : 'shell-nav-link'} href={href} aria-current={active(href) ? 'page' : undefined} title={compact ? label : undefined}>
      <Icon aria-hidden="true" size={17} />
      {!compact && <span><strong>{label}</strong></span>}
      {!compact && active(href) && <ChevronRight aria-hidden="true" size={15} />}
    </a>)}
  </nav>
}

function RuntimeSummary({ status }: { status: Status }) {
  const activeIndex = status.index.state === 'building' || status.index.state === 'catching_up'
  const complete = status.index.inventoriedCount ? Math.min(100, Math.round((status.index.processedCount + status.index.skippedCount + status.index.failedCount) / status.index.inventoriedCount * 100)) : 0
  return <section className="shell-runtime" aria-label="Local runtime status">
    <div className="shell-runtime-line"><span className={activeIndex ? 'runtime-dot active' : 'runtime-dot'} aria-hidden="true" /><span>{activeIndex ? 'Indexing local history' : status.index.state === 'current' ? 'Local index current' : status.index.state}</span></div>
    {activeIndex && <><Progress value={complete} aria-label={`Index progress ${complete}%`} /><small>{status.index.processedCount} processed · {status.index.remainingCount} remaining</small></>}
    {!activeIndex && <small>{status.index.supportedSourceCount} supported · {status.index.unsupportedSourceCount} limited</small>}
  </section>
}

export function AppShell({ status, children }: Props) {
  const [collapsed, setCollapsed] = React.useState(false)
  // Older saved test snapshots predate the explicit source-home status field.
  // Runtime responses always include it; the fallback keeps the shell readable
  // rather than fabricating a filesystem path.
  const sourceHome = status.sourceHome ?? { path: 'Source home unavailable', resolution: 'default' as const }
  const inspectorHome = status.inspectorHome?.path ?? 'Inspector home unavailable'
  const toggleSidebar = React.useCallback((event: React.MouseEvent<HTMLElement>) => {
    const target = event.target as HTMLElement
    if (target.closest('a,button,[role="button"],input,select,textarea')) return
    setCollapsed(value => !value)
  }, [])
  return <TooltipProvider><div className={collapsed ? 'app-shell collapsed' : 'app-shell'} onClick={handleInternalLinkClick}>
    <aside className="shell-sidebar" onClick={toggleSidebar} title={collapsed ? 'Click to expand sidebar' : 'Click empty sidebar space to collapse'}>
      <div className="shell-brand"><img className="brand-mark" src="/assets/codex-inspector-logo.png" alt="" />{!collapsed && <span><strong>Codex Inspector</strong></span>}</div>
      <Nav compact={collapsed} />
      {!collapsed && <RuntimeSummary status={status} />}
      <Tooltip><TooltipTrigger asChild><Button className="shell-collapse" variant="ghost" size="icon" onClick={() => setCollapsed(value => !value)} aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}>{collapsed ? <PanelLeftOpen /> : <PanelLeftClose />}</Button></TooltipTrigger><TooltipContent>{collapsed ? 'Expand sidebar' : 'Collapse sidebar'}</TooltipContent></Tooltip>
    </aside>
    <div className="shell-content">
      <header className="shell-mobile-header"><Sheet><SheetTrigger asChild><Button variant="outline" size="icon" aria-label="Open navigation"><Menu /></Button></SheetTrigger><SheetContent side="left" className="shell-mobile-sheet"><div className="shell-brand"><img className="brand-mark" src="/assets/codex-inspector-logo.png" alt="" /><span><strong>Codex Inspector</strong><small>Local observability</small></span></div><Nav /><RuntimeSummary status={status} /></SheetContent></Sheet><Badge variant="outline">{status.index.state}</Badge></header>
      <div className="shell-dataset" aria-label="Effective local homes">
        <div className="shell-dataset-item">
          <div className="shell-dataset-label"><span>CODEX_HOME</span><Badge className="shell-dataset-source" variant="secondary">{sourceHome.resolution === 'environment' ? 'from environment' : 'using default'}</Badge></div>
          <strong title={sourceHome.path}>{sourceHome.path}</strong>
        </div>
        <div className="shell-dataset-item">
          <div className="shell-dataset-label"><span>CODEX_INSPECTOR_HOME</span></div>
          <strong title={inspectorHome}>{inspectorHome}</strong>
        </div>
      </div>
      {children}
    </div>
  </div></TooltipProvider>
}
