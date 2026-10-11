export function HelpPage() {
  return <section className="task-surface" aria-label="Dispatch help">
    <header className="task-heading"><div><h1>Help</h1><p>Prepare, validate, and apply a solution with Dispatch’s local workspace.</p></div></header>
    <section className="plan-group"><h3>Start a deployment</h3><ol><li>Choose a solution and prepare its package.</li><li>Connect and verify the selected Box account and Salesforce environment.</li><li>Configure the strategy and included providers. The prepared package defines component scope; individual component exclusions are not supported here.</li><li>Review the plan, validate it, and inspect any changes before confirming deployment.</li></ol></section>
    <section className="plan-group"><h3>Recover a failed or interrupted run</h3><p>Keep the local Dispatch service running while work is active. If live updates disconnect, Dispatch checks the run status and reconnects.</p><p>For a failed run, open diagnostic guidance, edit connections if needed, and retry validation. A fresh validation is required before applying another attempt. History retains previous records.</p></section>
    <section className="plan-group"><h3>Manage connections</h3><p>Use Settings to manage saved connections and workspace defaults. Complete login in the provider window. If the window is closed or login times out, start login again. Closing the connection drawer stops its status watcher; it does not revoke authorization already completed with the provider.</p></section>
    <section className="plan-group"><h3>Local service unavailable</h3><p>Reopen Dispatch or wait for the service to restart, then use Retry loading or Retry diagnostics. Unavailable data does not mean that saved records were deleted.</p></section>
  </section>
}
