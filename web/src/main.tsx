import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import '@unofficialbox/box-open-elements/button'
import { applyDesignTokens, registerBoxDefaultDesignSystem } from '@unofficialbox/box-open-elements/foundations/tokens'
import './index.css'
import App from './App.tsx'

registerBoxDefaultDesignSystem({ setActive: true })
applyDesignTokens(document.documentElement, 'box-default')

createRoot(document.getElementById('root')!).render(
  <StrictMode><App /></StrictMode>,
)
