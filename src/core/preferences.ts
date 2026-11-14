import { tos } from './app.ts'
import { t } from './i18n.ts'

export { browserLocale, parseDesktopTheme, parseDesktopLanguage, parsePreferences, readDesktopThemeCookie, readDesktopTheme, readDesktopLanguage } from 'tos-app-core'
export type { Theme, ThemePreference, LanguagePreference } from 'tos-app-core'
export const { themePreference, languagePreference, theme, themeSource, languageSource } = tos
export function initializePreferences(): () => void {
  return tos.start({ title: () => t('批量重命名') })
}
