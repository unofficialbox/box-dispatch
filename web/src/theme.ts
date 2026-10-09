import { createThemeController, type ThemeController } from '@unofficialbox/box-open-elements/foundations/theming'

export const startDispatchTheme = (): ThemeController => {
  const controller = createThemeController({ preference: 'system' })
  controller.start()
  return controller
}
