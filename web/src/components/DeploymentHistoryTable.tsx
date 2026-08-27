import type { DeploymentSummary } from '../types'
import { deploymentOutcome, displayProvider, displayStrategy, formatDeploymentDate } from '../deploymentPresentation'

export function DeploymentHistoryTable({ deployments, caption, includeResult = false }: { deployments: DeploymentSummary[]; caption: string; includeResult?: boolean }) {
  const columns = includeResult ? 5 : 4
  return <div className="deployment-history-table"><table><caption className="visually-hidden">{caption}</caption><thead><tr><th scope="col">Deployment</th><th scope="col">Systems</th><th scope="col">Strategy</th>{includeResult ? <th scope="col">Result</th> : null}<th scope="col">Completed</th></tr></thead><tbody>{deployments.length ? deployments.map((deployment) => {
    const outcome = deploymentOutcome(deployment)
    return <tr key={deployment.id}><th scope="row">{deployment.name || deployment.id}</th><td>{deployment.providers.length ? deployment.providers.map((provider) => displayProvider(provider.name)).join(', ') : 'Not recorded'}</td><td>{displayStrategy(deployment.strategy)}</td>{includeResult ? <td><box-badge label={outcome.label} tone={outcome.tone}></box-badge></td> : null}<td>{formatDeploymentDate(deployment.completedAt)}</td></tr>
  }) : <tr><td colSpan={columns} className="history-empty">Completed deployments will appear here.</td></tr>}</tbody></table></div>
}
