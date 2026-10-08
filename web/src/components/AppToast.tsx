import { useEffect, useRef } from 'react'
import { Toast, type ToastRef } from '@unofficialbox/box-open-elements-react/toast'

function showToastInTopLayer(toast: ToastRef) {
  if (typeof toast.showPopover !== 'function') return
  try {
    if (!toast.matches(':popover-open')) toast.showPopover()
  } catch {
    /* already open */
  }
}

function hideToastFromTopLayer(toast: ToastRef) {
  if (typeof toast.hidePopover !== 'function') return
  try {
    if (toast.matches(':popover-open')) toast.hidePopover()
  } catch {
    /* already closed */
  }
}

export type AppToastNotice = {
  id: number
  message: string
  tone?: string
}

const toastDurationMs = 4000

export function AppToast({ notice, onDismiss }: { notice: AppToastNotice; onDismiss?: () => void }) {
  const ref = useRef<ToastRef>(null)
  useEffect(() => {
    const toast = ref.current
    if (!toast) return
    const frame = window.requestAnimationFrame(() => showToastInTopLayer(toast))
    return () => {
      window.cancelAnimationFrame(frame)
      hideToastFromTopLayer(toast)
    }
  }, [notice])
  return <Toast ref={ref} className="app-toast" popover="manual" message={notice.message} tone={notice.tone ?? 'success'} mode="dismissible" duration={toastDurationMs} open onOpenChanged={(event) => { if (!event.detail.open) onDismiss?.() }}/>
}
