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
    }
  }
}
