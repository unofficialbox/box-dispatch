import type { ReactNode } from 'react'
import '@unofficialbox/box-open-elements/fact-list'

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
  return <box-fact-list className="detail-list" rows={rows.map(([label, value]) => ({ label, value }))}></box-fact-list>
}
