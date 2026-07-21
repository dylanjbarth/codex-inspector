import type { MouseEvent as ReactMouseEvent } from 'react'

export function navigate(path: string) {
  history.pushState(null, '', path)
  dispatchEvent(new PopStateEvent('popstate'))
}

export function handleInternalLinkClick(event: ReactMouseEvent<HTMLElement>) {
  if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return

  const target = event.target
  if (!(target instanceof Element)) return
  const anchor = target.closest<HTMLAnchorElement>('a[href]')
  if (!anchor || anchor.hasAttribute('download') || (anchor.target && anchor.target !== '_self')) return

  const url = new URL(anchor.href, location.href)
  if (url.origin !== location.origin || (url.protocol !== 'http:' && url.protocol !== 'https:')) return
  if (url.pathname === location.pathname && url.search === location.search && url.hash) return

  event.preventDefault()
  if (url.href !== location.href) navigate(`${url.pathname}${url.search}${url.hash}`)
}
