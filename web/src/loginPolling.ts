// Watch only the local login job. Closing the drawer aborts this watcher;
// it does not revoke an authorization already completed in the provider window.
export async function watchLoginJob<T extends { status: string; message: string }>(url: string, popup: Pick<Window, 'closed'>, controller: AbortController, onJob: (job: T) => Promise<void> | void): Promise<void> {
  const stop = new AbortController()
  const cancel = () => stop.abort(controller.signal.reason)
  controller.signal.addEventListener('abort', cancel, { once: true })
  if (controller.signal.aborted) cancel()
  const { signal } = stop
  const closedWindow = window.setInterval(() => {
    if (popup.closed) stop.abort(new Error('Login window closed. Start login again when you are ready.'))
  }, 300)
  const timeout = window.setTimeout(() => stop.abort(new Error('Login timed out. Start login again.')), 120_000)
  try {
    while (!signal.aborted) {
      const response = await fetch(url, { signal })
      if (!response.ok) throw new Error('Login status is unavailable. Start login again.')
      const job = await response.json() as T
      if (signal.aborted) throw signal.reason
      if (job.status !== 'pending') {
        window.clearInterval(closedWindow)
        window.clearTimeout(timeout)
      }
      await onJob(job)
      if (job.status !== 'pending') return
      await new Promise<void>((resolve, reject) => {
        const abort = () => { window.clearTimeout(timer); reject(signal.reason) }
        const timer = window.setTimeout(() => { signal.removeEventListener('abort', abort); resolve() }, 1000)
        signal.addEventListener('abort', abort, { once: true })
        if (signal.aborted) { signal.removeEventListener('abort', abort); abort() }
      })
    }
    throw signal.reason
  } catch (error) {
    // Callback pages close after success. Recheck once before treating a closed
    // window as cancellation, even if its status request was interrupted.
    if (!controller.signal.aborted && popup.closed) {
      window.clearInterval(closedWindow)
      window.clearTimeout(timeout)
      const finalCheck = new AbortController()
      const cancelFinal = () => finalCheck.abort(controller.signal.reason)
      controller.signal.addEventListener('abort', cancelFinal, { once: true })
      const finalTimeout = window.setTimeout(() => finalCheck.abort(), 5000)
      try {
        const response = await fetch(url, { signal: finalCheck.signal })
        if (response.ok) {
          const job = await response.json() as T
          if (!controller.signal.aborted && job.status !== 'pending') {
            await onJob(job)
            return
          }
        }
      } catch { /* Keep the original cancellation guidance. */ }
      finally { window.clearTimeout(finalTimeout); controller.signal.removeEventListener('abort', cancelFinal) }
    }
    throw error
  } finally {
    controller.signal.removeEventListener('abort', cancel)
    window.clearInterval(closedWindow)
    window.clearTimeout(timeout)
  }
}
