import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import '@unofficialbox/box-open-elements/button'
import '@unofficialbox/box-open-elements/card'
import '@unofficialbox/box-open-elements/switch'
import '@unofficialbox/box-open-elements/badge'
import '@unofficialbox/box-open-elements/progress-bar'
import '@unofficialbox/box-open-elements/drawer'
import '@unofficialbox/box-open-elements/text-field'
import '@unofficialbox/box-open-elements/select'
import '@unofficialbox/box-open-elements/split-view'
import { applyDesignTokens, registerBoxDefaultDesignSystem } from '@unofficialbox/box-open-elements/foundations/tokens'
import './index.css'
import App from './App.tsx'

registerBoxDefaultDesignSystem({ setActive: true })
applyDesignTokens(document.documentElement, 'box-default')

createRoot(document.getElementById('root')!).render(
  <StrictMode><App /></StrictMode>,
)
