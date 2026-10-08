import { useEffect, useRef, useState } from 'react'
import '@unofficialbox/box-open-elements/nav-sidebar'
import '@unofficialbox/box-open-elements/sidebar-toggle-button'
import { RailIcon } from './RailIcon'

export type AppView = 'overview' | 'workflow' | 'history' | 'settings'

export function Sidebar({ activeView, onOverview, onNewDeployment, onHistory, onSettings }: { activeView: AppView; onOverview: () => void; onNewDeployment: () => void; onHistory: () => void; onSettings: () => void }) {
  const [collapsed, setCollapsed] = useState(true)
  const toggleRef = useRef<HTMLElement | null>(null)

  useEffect(() => {
    const toggle = toggleRef.current
    if (!toggle) return
    const handleToggle = (event: Event) => setCollapsed(!(event as CustomEvent<{ expanded: boolean }>).detail.expanded)
    toggle.addEventListener('toggle', handleToggle)
    return () => toggle.removeEventListener('toggle', handleToggle)
  }, [])

  return <box-nav-sidebar id="dispatch-navigation" className="sidebar" slot="nav" label="Dispatch routes" collapsed={collapsed}>
    <button className="brand" type="button" onClick={onOverview} aria-label="Box Dispatch overview"><span className="brand-icon" aria-hidden="true">B/</span><span className="brand-name" data-nav-label>Dispatch</span></button>
    <button className={`nav-link nav-button ${activeView === 'overview' ? 'active' : ''}`} type="button" onClick={onOverview} aria-current={activeView === 'overview' ? 'page' : undefined} aria-label="Overview" title="Overview"><span data-nav-icon><RailIcon name="grid" /></span><span data-nav-label>Overview</span></button>
    <button className={`nav-link nav-button ${activeView === 'workflow' ? 'active' : ''}`} type="button" onClick={onNewDeployment} aria-current={activeView === 'workflow' ? 'page' : undefined} aria-label="Deployments" title="Deployments"><span data-nav-icon><RailIcon name="rocket" /></span><span data-nav-label>Deployments</span></button>
    <button className={`nav-link nav-button ${activeView === 'history' ? 'active' : ''}`} type="button" onClick={onHistory} aria-current={activeView === 'history' ? 'page' : undefined} aria-label="Deployment history" title="History"><span data-nav-icon><RailIcon name="clock2" /></span><span data-nav-label>History</span></button>
    <button className={`nav-link nav-button ${activeView === 'settings' ? 'active' : ''}`} type="button" onClick={onSettings} aria-current={activeView === 'settings' ? 'page' : undefined} aria-label="Settings" title="Settings"><span data-nav-icon><RailIcon name="settings" /></span><span data-nav-label>Settings</span></button>
    <box-sidebar-toggle-button ref={toggleRef} className="sidebar-toggle" controls="dispatch-navigation" label={collapsed ? 'Expand navigation' : 'Collapse navigation'} expanded={!collapsed}></box-sidebar-toggle-button>
    <a className="nav-link nav-help" href="#help" aria-label="Help" title="Help"><span data-nav-icon><RailIcon name="help" /></span><span data-nav-label>Help</span></a>
  </box-nav-sidebar>
}
