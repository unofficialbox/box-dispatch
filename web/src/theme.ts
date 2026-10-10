import { createThemeController, type ThemeController } from '@unofficialbox/box-open-elements/foundations/theming'
import {
  boxDarkDesignSystem,
  boxDefaultDesignSystem,
  registerDesignSystem,
} from '@unofficialbox/box-open-elements/foundations/tokens'

export const DISPATCH_LIGHT_THEME = 'dispatch-light'
export const DISPATCH_DARK_THEME = 'dispatch-dark'

const registerDispatchDesignSystems = () => {
  registerDesignSystem({
    ...boxDefaultDesignSystem,
    name: DISPATCH_LIGHT_THEME,
    tokens: {
      ...boxDefaultDesignSystem.tokens,
      SurfaceSurfaceBrand: '#007a4c',
      SurfaceSurfaceBrandHover: '#006b43',
      SurfaceSurfaceBrandPressed: '#005c39',
      SurfaceItemSurfaceSelected: '#e6f7f0',
      SurfaceItemSurfaceHover: '#eefaf5',
      SurfaceStatusSurfaceInprogress: '#ffa300',
      SurfaceStatusSurfaceAccent: '#8b49cf',
      SurfaceIllustrationSurfaceBoxNeutral: '#007a4c',
      SurfaceBadgeFoldersharedSurface: '#0098ff',
      TextStatusTextWarning: '#7a4a00',
    },
  })

  registerDesignSystem({
    ...boxDarkDesignSystem,
    name: DISPATCH_DARK_THEME,
    tokens: {
      ...boxDarkDesignSystem.tokens,
      SurfaceSurface: '#121513',
      SurfaceSurfaceHover: '#1d211f',
      SurfaceSurfaceSecondary: '#0c0f0d',
      SurfaceSurfaceTertiary: '#2a302d',
      SurfaceSurfaceQuaternary: '#3a423e',
      SurfaceSurfaceBrand: '#00e581',
      SurfaceSurfaceBrandHover: '#21f093',
      SurfaceSurfaceBrandPressed: '#00c971',
      SurfaceSearchSurface: '#1d211f',
      SurfaceItemSurfaceSelected: '#153d31',
      SurfaceItemSurfaceHover: '#1a3028',
      SurfaceTooltipSurface: '#333a36',
      SurfaceStatusSurfaceInprogress: '#ffa300',
      SurfaceStatusSurfaceAccent: '#8b49cf',
      SurfaceIllustrationSurfaceBoxNeutral: '#00e581',
      SurfaceBadgeFoldersharedSurface: '#0098ff',
      TextTextOnBrand: '#07100c',
      TextStatusTextWarning: '#ffa300',
      StrokeStroke: '#343a37',
      StrokeStrokeHover: '#4b5550',
    },
  })
}

export const startDispatchTheme = (): ThemeController => {
  registerDispatchDesignSystems()
  const controller = createThemeController({
    preference: 'system',
    lightDesignSystem: DISPATCH_LIGHT_THEME,
    darkDesignSystem: DISPATCH_DARK_THEME,
  })
  controller.start()
  return controller
}
