import type * as React from 'react'

declare module 'react' {
  namespace JSX {
    interface IntrinsicElements {
      'box-app-shell': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        heading?: string
        navLabel?: string
        asideLabel?: string
      }
      'box-nav-sidebar': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        collapsed?: boolean
        label?: string
      }
      'box-sidebar-toggle-button': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        controls?: string
        direction?: 'left' | 'right'
        disabled?: boolean
        expanded?: boolean
        label?: string
      }
      'box-breadcrumb': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        items?: import('@unofficialbox/box-open-elements/breadcrumb').BreadcrumbItem[]
        label?: string
        maxItems?: number
      }
      'box-path': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        stages?: import('@unofficialbox/box-open-elements/path').PathStage[]
        label?: string
        current?: string
        variant?: import('@unofficialbox/box-open-elements/path').PathVariant
        hasError?: boolean
      }
      'box-resource-row': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        label?: string
        meta?: string
        status?: string
        value?: string
        selected?: boolean
        active?: boolean
        disabled?: boolean
      }
      'box-button': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        label?: string
        tone?: 'primary' | 'neutral' | 'danger'
        disabled?: boolean
        isLoading?: boolean
      }
      'box-link-button': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        href?: string
        label?: string
        rel?: string
        target?: string
        tone?: 'primary' | 'neutral' | 'danger'
      }
      'box-accordion': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        borderless?: boolean
        items?: Array<{ content?: string; summary?: string; label: string; value: string }>
        label?: string
        multiple?: boolean
        plainPanels?: boolean
        value?: string
        values?: string[]
      }
      'box-icon-button': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        icon?: string
        label?: string
        tone?: 'primary' | 'secondary' | 'danger'
        disabled?: boolean
      }
      'box-card': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        eyebrow?: string
        heading?: string
      }
      'box-section': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        heading?: string
        eyebrow?: string
        description?: string
      }
      'box-fact-list': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        rows?: Array<{ label: string; value: string }>
      }
      'box-tile-group': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        legend?: string
        multiple?: boolean
        name?: string
        options?: import('@unofficialbox/box-open-elements/tile-group').TileOption[]
        value?: string
      }
      'box-search-field': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        label?: string
        value?: string
        placeholder?: string
        disabled?: boolean
        loading?: boolean
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
      'box-timeline': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        heading?: string
        events?: import('@unofficialbox/box-open-elements/patterns/timeline').TimelineEvent[]
        composable?: boolean
        hasMore?: boolean
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
        tone?: 'neutral' | 'info' | 'brand' | 'success' | 'error' | 'warning' | 'inprogress'
      }
      'box-status-icon': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        kind?: import('@unofficialbox/box-open-elements/foundations/status').StatusKind
        label?: string
      }
      'box-progress-bar': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        label?: string
        max?: number
        value?: number
      }
      'box-spinner': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        label?: string
        size?: 'small' | 'medium' | 'large'
      }
      'box-drawer': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        heading?: string
        description?: string
        open?: boolean
        position?: 'left' | 'right' | 'bottom'
        size?: 'small' | 'medium' | 'large' | 'full'
        busy?: boolean
        hideCloseButton?: boolean
      }
      'box-diff-viewer': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        heading?: string
        'before-text'?: string
        'after-text'?: string
        'before-label'?: string
        'after-label'?: string
        mode?: 'split' | 'inline'
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
      'box-toast': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        message?: string
        heading?: string
        open?: boolean
        tone?: string
        duration?: number
        mode?: 'dismissible' | 'sticky'
        borderless?: boolean
        popover?: 'auto' | 'manual' | 'hint'
      }
    }
  }
}
