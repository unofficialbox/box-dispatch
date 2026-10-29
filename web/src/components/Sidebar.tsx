import { useEffect, useRef, useState, type MouseEvent } from 'react'
import '@unofficialbox/box-open-elements/nav-sidebar'
import '@unofficialbox/box-open-elements/sidebar-toggle-button'
import type { BoxElementEventMap } from '@unofficialbox/box-open-elements/native-types'
import { RailIcon } from './RailIcon'

export type AppView = 'overview' | 'workflow' | 'compare' | 'history' | 'settings' | 'help'

export function Sidebar({ activeView, onOverview, onNewDeployment, onCompare, onHistory, onSettings }: { activeView: AppView; onOverview: () => void; onNewDeployment: () => void; onCompare: () => void; onHistory: () => void; onSettings: () => void }) {
  const [collapsed, setCollapsed] = useState(() => window.localStorage.getItem('dispatch-sidebar-collapsed') === 'true')
  const toggleRef = useRef<HTMLElementTagNameMap['box-sidebar-toggle-button'] | null>(null)

  useEffect(() => {
    const toggle = toggleRef.current
    if (!toggle) return
    const handleToggle = (event: BoxElementEventMap['box-sidebar-toggle-button']['toggle']) => {
      const nextCollapsed = !event.detail.expanded
      setCollapsed(nextCollapsed)
      window.localStorage.setItem('dispatch-sidebar-collapsed', String(nextCollapsed))
    }
    toggle.addEventListener('toggle', handleToggle)
    return () => toggle.removeEventListener('toggle', handleToggle)
  }, [])

  const route = (callback: () => void) => (event: MouseEvent<HTMLAnchorElement>) => { event.preventDefault(); callback() }
  return <box-nav-sidebar id="dispatch-navigation" className="sidebar" slot="nav" label="Box Dispatch navigation" collapsed={collapsed}>
    <a className="brand" slot="header" href="#" onClick={route(onOverview)} aria-label="Dispatch Solution delivery overview">
      <span className="brand-icon" aria-hidden="true">
        <svg viewBox="0 0 24 24" width="36" height="36">
          <path fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"
            d="M12 2l9 5v10l-9 5-9-5V7l9-5z"/>
          <path fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"
            d="M12 12l9-5M12 12v10M12 12L3 7"/>
        </svg>
      </span>
      {' '}
      <span className="brand-copy" data-nav-label><span className="brand-name">Dispatch</span>{' '}<small>Solution delivery</small></span>
    </a>
    <span className="nav-group-label" data-nav-group data-nav-label>Workspace</span>
    <a className={`nav-link nav-route ${activeView === 'overview' ? 'active' : ''}`} href="#" onClick={route(onOverview)} aria-current={activeView === 'overview' ? 'page' : undefined} aria-label="Overview"><span data-nav-icon><RailIcon name="grid" /></span><span data-nav-label>Overview</span></a>
    <a className={`nav-link nav-route ${activeView === 'compare' ? 'active' : ''}`} href="#compare" onClick={route(onCompare)} aria-current={activeView === 'compare' ? 'page' : undefined} aria-label="Compare Box environments"><span data-nav-icon><RailIcon name="compare" /></span><span data-nav-label>Compare</span></a>
    <a className={`nav-link nav-route ${activeView === 'workflow' ? 'active' : ''}`} href="#workspace" onClick={route(onNewDeployment)} aria-current={activeView === 'workflow' ? 'page' : undefined} aria-label="Deployments"><span data-nav-icon><RailIcon name="rocket" /></span><span data-nav-label>Deployments</span></a>
    <a className={`nav-link nav-route ${activeView === 'history' ? 'active' : ''}`} href="#history" onClick={route(onHistory)} aria-current={activeView === 'history' ? 'page' : undefined} aria-label="Deployment history"><span data-nav-icon><RailIcon name="clock2" /></span><span data-nav-label>History</span></a>
    <a className={`nav-link nav-route ${activeView === 'settings' ? 'active' : ''}`} href="#settings" onClick={route(onSettings)} aria-current={activeView === 'settings' ? 'page' : undefined} aria-label="Settings"><span data-nav-icon><RailIcon name="settings" /></span><span data-nav-label>Settings</span></a>
    <box-sidebar-toggle-button ref={toggleRef} className="sidebar-toggle" slot="header" label={collapsed ? 'Expand navigation' : 'Collapse navigation'} expanded={!collapsed}></box-sidebar-toggle-button>
    <a className={`nav-link nav-help ${activeView === 'help' ? 'active' : ''}`} aria-current={activeView === 'help' ? 'page' : undefined} href="#help" aria-label="Help" title="Help"><span data-nav-icon><RailIcon name="help" /></span><span data-nav-label>Help</span></a>
    <div className="sidebar-context" slot="footer"><strong>Local workspace</strong><span>Box + Salesforce</span></div>
  </box-nav-sidebar>
}
