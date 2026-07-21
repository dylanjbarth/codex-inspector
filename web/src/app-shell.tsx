import * as React from 'react'
import { BarChart3, BookOpenText, ChevronRight, Menu, RefreshCw } from 'lucide-react'
import type { Status } from './api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { Sheet, SheetContent, SheetTrigger } from '@/components/ui/sheet'
import { TooltipProvider } from '@/components/ui/tooltip'
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

function IndexRailStatus({ status }: { status: Status }) {
  const pass = status.index.activePass
  if (!pass) return <section className="shell-index-status idle" aria-label={`Local index ${status.index.state}, revision ${status.index.appliedRevision}`}>
    <span className="shell-index-signal" aria-hidden="true" />
    <span><b>Index</b><strong>{status.index.state === 'current' ? 'Current' : status.index.state.replaceAll('_', ' ')} · r{status.index.appliedRevision}</strong></span>
  </section>
  const handled = pass.phase === 'rebuilding' ? pass.scannedCount : pass.processedCount + pass.skippedCount + pass.failedCount + pass.requiresRebuildCount
  const complete = pass.inventoriedCount ? Math.min(100, Math.round(handled / pass.inventoriedCount * 100)) : 0
  const title = pass.phase === 'discovering' ? 'Discovering sources' : pass.phase === 'rebuilding' ? 'Rebuilding catalog' : pass.phase === 'finalizing' ? 'Finalizing snapshot' : 'Indexing history'
  const detail = pass.phase === 'discovering'
    ? `${status.index.queuedSessionChanges} queued change${status.index.queuedSessionChanges === 1 ? '' : 's'} waiting for this pass`
    : pass.phase === 'rebuilding'
      ? `${pass.scannedCount} of ${pass.inventoriedCount} sources prepared · upgrading the source adapter without replacing the active snapshot early`
    : pass.phase === 'finalizing'
      ? `${pass.inventoriedCount} sources checked · reconciling lineage and committing the snapshot`
      : `${handled} of ${pass.inventoriedCount} sources checked · ${pass.processedCount} indexed · ${pass.skippedCount} unchanged · ${pass.remainingCount} remaining`
  return <section className={`shell-index-status active ${pass.phase}`} role="status" aria-live="polite" aria-label={`${title}. ${detail}`} title={detail}>
    <span className="shell-index-signal" aria-hidden="true" />
    <span><b>Index</b><strong>{title}</strong></span>
    {pass.inventoriedCount > 0 ? <span className="shell-index-progress"><Progress value={complete} aria-label={`Current index pass ${complete}%`} /><em>{handled}/{pass.inventoriedCount} · {complete}%</em></span> : <span className="shell-index-scanning"><i aria-hidden="true" />Scanning…</span>}
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
    </aside>
    <div className="shell-content">
      <header className="shell-mobile-header"><Sheet><SheetTrigger asChild><Button variant="outline" size="icon" aria-label="Open navigation"><Menu /></Button></SheetTrigger><SheetContent side="left" className="shell-mobile-sheet"><div className="shell-brand"><img className="brand-mark" src="/assets/codex-inspector-logo.png" alt="" /><span><strong>Codex Inspector</strong><small>Local observability</small></span></div><Nav /></SheetContent></Sheet><Badge variant="outline">{status.index.state}</Badge></header>
      <div className="shell-dataset" aria-label="Effective local homes">
        <div className="shell-dataset-item">
          <div className="shell-dataset-label"><span>CODEX_HOME</span><Badge className="shell-dataset-source" variant="secondary">{sourceHome.resolution === 'environment' ? 'from environment' : 'using default'}</Badge></div>
          <strong title={sourceHome.path}>{sourceHome.path}</strong>
        </div>
        <div className="shell-dataset-item">
          <div className="shell-dataset-label"><span>CODEX_INSPECTOR_HOME</span></div>
          <strong title={inspectorHome}>{inspectorHome}</strong>
        </div>
        <IndexRailStatus status={status} />
      </div>
      {children}
    </div>
  </div></TooltipProvider>
}
