import { boxIconography } from '@unofficialbox/box-open-elements/foundations/icons'

export function RailIcon({ name }: { name: 'grid' | 'rocket' | 'document' | 'settings' | 'help' }) {
  const common = { fill: 'none', stroke: 'currentColor', strokeWidth: 1.8, strokeLinecap: 'round' as const, strokeLinejoin: 'round' as const }
  if (name === 'grid') return <svg viewBox="0 0 24 24" aria-hidden="true"><rect {...common} x="3" y="3" width="7" height="7"/><rect {...common} x="14" y="3" width="7" height="7"/><rect {...common} x="3" y="14" width="7" height="7"/><rect {...common} x="14" y="14" width="7" height="7"/></svg>
  if (name === 'rocket') return <svg viewBox="0 0 24 24" aria-hidden="true"><path {...common} d="M14.2 4.1c2.7-1.1 4.7-1 5.7-.7.3 1 .4 3-.7 5.7l-4.4 4.4-4.5-4.5 3.9-4.9Z"/><path {...common} d="m10.3 9.1-4.1.5-2.1 2.1 5.2.5M14.8 13.7l-.5 4.1-2.1 2.1-.5-5.2M8 16l-3 3M7.3 14.1l2.6 2.6"/><circle {...common} cx="15.7" cy="8.3" r="1.2"/></svg>
  if (name === 'document') return <svg viewBox="0 0 24 24" aria-hidden="true"><path {...common} d="M6 3h8l4 4v14H6z"/><path {...common} d="M14 3v5h5M9 12h6M9 16h6"/></svg>
  if (name === 'settings') return <span className="boe-rail-icon" aria-hidden="true" dangerouslySetInnerHTML={{ __html: boxIconography.gear }}/>
  return <svg viewBox="0 0 24 24" aria-hidden="true"><circle {...common} cx="12" cy="12" r="9"/><path {...common} d="M9.6 9a2.5 2.5 0 1 1 4.3 1.7c-.9.8-1.9 1.3-1.9 2.8M12 17h.01"/></svg>
}
