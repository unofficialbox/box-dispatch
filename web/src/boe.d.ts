import type * as React from 'react'

declare module 'react' {
  namespace JSX {
    interface IntrinsicElements {
      'box-button': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        label?: string
        tone?: 'primary' | 'secondary' | 'danger' | 'success'
        disabled?: boolean
        isLoading?: boolean
      }
      'box-card': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        eyebrow?: string
        heading?: string
      }
      'box-metric-card': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        eyebrow?: string
        heading?: string
        value?: string
        message?: string
        status?: string
        action?: { id: string; label: string; tone?: string } | null
        trend?: { direction?: 'up' | 'down' | 'flat'; label: string; tone?: string } | null
      }
      'box-run-trace': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        heading?: string
        steps?: import('@unofficialbox/box-open-elements/patterns/run').RunStep[]
      }
      'box-table': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        label?: string
        columns?: import('@unofficialbox/box-open-elements/table').TableColumn[]
        rows?: import('@unofficialbox/box-open-elements/table').TableRow[]
        loading?: boolean
        emptyText?: string
        errorText?: string
        selectionMode?: import('@unofficialbox/box-open-elements/table').TableSelectionMode
        selectedIds?: string[]
      }
      'box-switch': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        checked?: boolean
        disabled?: boolean
        label?: string
        description?: string
        value?: string
      }
      'box-badge': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        label?: string
        tone?: 'info' | 'success' | 'warning' | 'danger' | string
      }
      'box-progress-bar': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        label?: string
        max?: number
        value?: number
      }
      'box-drawer': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        heading?: string
        description?: string
        open?: boolean
        position?: 'left' | 'right' | 'bottom'
        size?: 'small' | 'medium' | 'large' | 'full'
        busy?: boolean
      }
      'box-text-field': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        label?: string
        value?: string
        type?: 'text' | 'email' | 'tel' | 'url' | 'password' | 'search' | 'number'
        placeholder?: string
        description?: string
        required?: boolean
        disabled?: boolean
        loading?: boolean
        valid?: boolean
        invalid?: boolean
        errorMessage?: string
        hideLabel?: boolean
        autocomplete?: string
        reveal?: boolean
      }
      'box-select': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        label?: string
        value?: string
        values?: string[]
        options?: Array<{ label: string; value: string; disabled?: boolean; group?: string }>
        description?: string
        required?: boolean
        disabled?: boolean
        multiple?: boolean
        invalid?: boolean
        errorMessage?: string
        hideLabel?: boolean
        loading?: boolean
        emptyText?: string
      }
      'box-split-view': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        label?: string
        ratio?: number
        resizable?: boolean
      }
    }
  }
}
