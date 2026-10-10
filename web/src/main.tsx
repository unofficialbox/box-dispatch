import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import type {} from '@unofficialbox/box-open-elements/react-jsx'
import '@unofficialbox/box-open-elements/button'
import '@unofficialbox/box-open-elements/icon-button'
import '@unofficialbox/box-open-elements/card'
import '@unofficialbox/box-open-elements/switch'
import '@unofficialbox/box-open-elements/badge'
import '@unofficialbox/box-open-elements/progress-bar'
import '@unofficialbox/box-open-elements/spinner'
import '@unofficialbox/box-open-elements/drawer'
import '@unofficialbox/box-open-elements/text-field'
import '@unofficialbox/box-open-elements/select'
import '@unofficialbox/box-open-elements/split-view'
import '@unofficialbox/box-open-elements/metric-card'
import '@unofficialbox/box-open-elements/run-trace'
import '@unofficialbox/box-open-elements/toast'
import './index.css'
import App from './App.tsx'
import { startDispatchTheme } from './theme.ts'

startDispatchTheme()

createRoot(document.getElementById('root')!).render(
  <StrictMode><App /></StrictMode>,
)
