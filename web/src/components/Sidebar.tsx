import { RailIcon } from './RailIcon'

export function Sidebar({ activeView, onOverview, onNewDeployment }: { activeView: 'overview' | 'workflow'; onOverview: () => void; onNewDeployment: () => void }) {
  return <aside className="sidebar" aria-label="Main navigation">
    <button className="brand" type="button" onClick={onOverview} aria-label="Box Dispatch overview"><span className="brand-icon" aria-hidden="true">B/</span><span className="brand-name">Dispatch</span></button>
    <nav>
      <button className={`nav-link nav-button ${activeView === 'overview' ? 'active' : ''}`} type="button" onClick={onOverview} aria-label="Overview" title="Overview"><RailIcon name="grid" /><span>Overview</span></button>
      <button className={`nav-link nav-button ${activeView === 'workflow' ? 'active' : ''}`} type="button" onClick={onNewDeployment} aria-label="Deployments" title="Deployments"><RailIcon name="rocket" /><span>Deployments</span></button>
      <a className="nav-link" href="#history" aria-label="Deployment history" title="History"><RailIcon name="document" /><span>History</span></a>
      <a className="nav-link" href="#settings" aria-label="Settings" title="Settings"><RailIcon name="settings" /><span>Settings</span></a>
    </nav>
    <a className="nav-link nav-help" href="#help" aria-label="Help" title="Help"><RailIcon name="help" /><span>Help</span></a>
  </aside>
}
