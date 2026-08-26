import type { ReactNode } from 'react'

export function DetailsRail({ title, description, children }: { title: string; description?: string; children: ReactNode }) {
  return <aside className="details-rail" aria-live="polite">
    <box-card>
      <section>
        <h2>{title}</h2>
        {description ? <p>{description}</p> : null}
        <div className="details-rail-content">{children}</div>
      </section>
    </box-card>
  </aside>
}

export function DetailList({ rows }: { rows: Array<[string, string]> }) {
  return <dl className="detail-list">
    {rows.map(([label, value]) => {
      const valueClass = value.toLowerCase().replaceAll(/[^a-z0-9]+/g, '-')
      return <div className={`detail-row detail-row-${label.toLowerCase().replaceAll(/[^a-z0-9]+/g, '-')}`} key={label}><dt>{label}</dt><dd className={`detail-value detail-value-${valueClass}`}>{value}</dd></div>
    })}
  </dl>
}
