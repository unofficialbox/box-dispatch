import { useEffect, useRef } from 'react'

type ToastElement = HTMLElement & {
  show: (message?: string, options?: { duration?: number; tone?: string }) => void
  hide: () => void
  showPopover?: () => void
  hidePopover?: () => void
}

function showToastInTopLayer(toast: ToastElement) {
  if (typeof toast.showPopover !== 'function') return
  try {
    if (!toast.matches(':popover-open')) toast.showPopover()
  } catch {
    /* already open */
  }
}

function hideToastFromTopLayer(toast: ToastElement) {
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
  const ref = useRef<ToastElement>(null)
  const onDismissRef = useRef(onDismiss)
  useEffect(() => {
    onDismissRef.current = onDismiss
  }, [onDismiss])
  useEffect(() => {
    const toast = ref.current
    if (!toast) return
    const handleDismiss = () => onDismissRef.current?.()
    const handleOpenChanged = (event: Event) => {
      if (!(event as CustomEvent<{ open: boolean }>).detail.open) handleDismiss()
    }
    toast.addEventListener('dismiss', handleDismiss)
    toast.addEventListener('open-changed', handleOpenChanged)
    toast.show(notice.message, { tone: notice.tone ?? 'success', duration: toastDurationMs })
    const frame = window.requestAnimationFrame(() => showToastInTopLayer(toast))
    const timeout = window.setTimeout(handleDismiss, toastDurationMs)
    return () => {
      toast.removeEventListener('dismiss', handleDismiss)
      toast.removeEventListener('open-changed', handleOpenChanged)
      window.cancelAnimationFrame(frame)
      window.clearTimeout(timeout)
      hideToastFromTopLayer(toast)
    }
  }, [notice])
  return <box-toast ref={ref} className="app-toast" popover="manual" message={notice.message} tone={notice.tone ?? 'success'} mode="dismissible" borderless></box-toast>
}
