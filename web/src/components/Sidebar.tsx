import { useEffect, useRef, useState, type MouseEvent } from 'react'
import '@unofficialbox/box-open-elements/nav-sidebar'
import '@unofficialbox/box-open-elements/sidebar-toggle-button'
import { RailIcon } from './RailIcon'

export type AppView = 'overview' | 'workflow' | 'history' | 'settings'

export function Sidebar({ activeView, onOverview, onNewDeployment, onHistory, onSettings }: { activeView: AppView; onOverview: () => void; onNewDeployment: () => void; onHistory: () => void; onSettings: () => void }) {
  const [collapsed, setCollapsed] = useState(() => window.localStorage.getItem('dispatch-sidebar-collapsed') === 'true')
  const toggleRef = useRef<HTMLElement | null>(null)

  useEffect(() => {
    const toggle = toggleRef.current
    if (!toggle) return
    const handleToggle = (event: Event) => {
      const nextCollapsed = !(event as CustomEvent<{ expanded: boolean }>).detail.expanded
      setCollapsed(nextCollapsed)
      window.localStorage.setItem('dispatch-sidebar-collapsed', String(nextCollapsed))
    }
    toggle.addEventListener('toggle', handleToggle)
    return () => toggle.removeEventListener('toggle', handleToggle)
  }, [])

  const route = (callback: () => void) => (event: MouseEvent<HTMLAnchorElement>) => { event.preventDefault(); callback() }
  return <box-nav-sidebar id="dispatch-navigation" className="sidebar" slot="nav" label="Box Dispatch navigation" collapsed={collapsed}>
    <a className="brand" slot="header" href="#" onClick={route(onOverview)} aria-label="Box Dispatch overview"><span className="brand-icon" aria-hidden="true">B/</span><span className="brand-copy" data-nav-label><span className="brand-name">Dispatch</span><small>Solution delivery</small></span></a>
    <span className="nav-group-label" data-nav-group data-nav-label>Workspace</span>
    <a className={`nav-link nav-route ${activeView === 'overview' ? 'active' : ''}`} href="#" onClick={route(onOverview)} aria-current={activeView === 'overview' ? 'page' : undefined}><span data-nav-icon><RailIcon name="grid" /></span><span data-nav-label>Overview</span></a>
    <a className={`nav-link nav-route ${activeView === 'workflow' ? 'active' : ''}`} href="#workspace" onClick={route(onNewDeployment)} aria-current={activeView === 'workflow' ? 'page' : undefined}><span data-nav-icon><RailIcon name="rocket" /></span><span data-nav-label>Deployments</span></a>
    <a className={`nav-link nav-route ${activeView === 'history' ? 'active' : ''}`} href="#history" onClick={route(onHistory)} aria-current={activeView === 'history' ? 'page' : undefined} aria-label="Deployment history"><span data-nav-icon><RailIcon name="clock2" /></span><span data-nav-label>History</span></a>
    <a className={`nav-link nav-route ${activeView === 'settings' ? 'active' : ''}`} href="#settings" onClick={route(onSettings)} aria-current={activeView === 'settings' ? 'page' : undefined}><span data-nav-icon><RailIcon name="settings" /></span><span data-nav-label>Settings</span></a>
    <box-sidebar-toggle-button ref={toggleRef} className="sidebar-toggle" slot="header" controls="dispatch-navigation" label={collapsed ? 'Expand navigation' : 'Collapse navigation'} expanded={!collapsed}></box-sidebar-toggle-button>
    <a className="nav-link nav-help" href="#help" aria-label="Help" title="Help"><span data-nav-icon><RailIcon name="help" /></span><span data-nav-label>Help</span></a>
    <div className="sidebar-context" slot="footer"><strong>Local workspace</strong><span>Box + Salesforce</span></div>
  </box-nav-sidebar>
}
