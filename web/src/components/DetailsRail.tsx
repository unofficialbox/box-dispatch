import type { ReactNode } from 'react'

export function DetailsRail({ title, description, children }: { title: string; description: string; children: ReactNode }) {
  return <aside className="details-rail" aria-live="polite">
    <box-card>
      <section>
        <h2>{title}</h2>
        <p>{description}</p>
        <div className="details-rail-content">{children}</div>
      </section>
    </box-card>
  </aside>
}

export function DetailList({ rows }: { rows: Array<[string, string]> }) {
  return <dl className="detail-list">
    {rows.map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}
  </dl>
}
