import { computed, ref, watch } from 'vue'
import { locale, t } from './i18n.ts'
import type { Locale } from './i18n.ts'

export type Theme = 'light' | 'dark'
export type ThemePreference = 'tos' | 'browser' | Theme
export type LanguagePreference = 'tos' | 'browser' | Locale
export const themePreference = ref<ThemePreference>('tos')
export const languagePreference = ref<LanguagePreference>('tos')
const browserDark = ref(false)
const browserLanguage = ref<Locale>('zh-CN')
const desktopTheme = ref<Theme | null>(null)
const desktopLanguage = ref<Locale | null>(null)
const browserTheme = computed<Theme>(() => browserDark.value ? 'dark' : 'light')
export const theme = computed(() => themePreference.value === 'tos' ? desktopTheme.value ?? browserTheme.value : themePreference.value === 'browser' ? browserTheme.value : themePreference.value)
const language = computed(() => languagePreference.value === 'tos' ? desktopLanguage.value ?? browserLanguage.value : languagePreference.value === 'browser' ? browserLanguage.value : languagePreference.value)
export const themeSource = computed(() => themePreference.value === 'tos' && desktopTheme.value ? 'tos' : 'browser')
export const languageSource = computed(() => languagePreference.value === 'tos' && desktopLanguage.value ? 'tos' : 'browser')
const storageKey = 'batch-rename.preferences'

export function browserLocale(languages: readonly string[]): Locale {
  for (const language of languages) {
    if (/^zh(?:[-_]|$)/i.test(language)) return 'zh-CN'
    if (/^en(?:[-_]|$)/i.test(language)) return 'en-US'
  }
  return 'en-US'
}

export function parseDesktopTheme(payload: unknown): Theme | null {
  if (!payload || typeof payload !== 'object') return null
  const response = payload as { code?: unknown; is_login?: unknown; data?: { display_mode?: unknown } }
  if (response.code !== true || response.is_login === false) return null
  const mode = response.data?.display_mode
  return mode === 'light' || mode === 'dark' ? mode : null
}

export function parseDesktopLanguage(payload: unknown): Locale | null {
  if (!payload || typeof payload !== 'object') return null
  const response = payload as { code?: unknown; is_login?: unknown; data?: { tos_lang?: unknown } }
  if (response.code !== true || response.is_login === false || typeof response.data?.tos_lang !== 'string') return null
  const value = response.data.tos_lang
  if (/^zh(?:[-_]|$)/i.test(value)) return 'zh-CN'
  if (/^[a-z]{2,3}(?:[-_][a-z0-9]{2,8})*$/i.test(value)) return 'en-US'
  return null
}

export function readDesktopThemeCookie(): Theme | null {
  try {
    const value = document.cookie.split(';').map(part => part.trim()).find(part => part.startsWith('themeMode='))?.slice('themeMode='.length)
    return value === 'light' || value === 'dark' ? value : null
  } catch { return null }
}

export function parsePreferences(raw: string | null): { theme: ThemePreference; language: LanguagePreference } {
  try {
    const value = JSON.parse(raw ?? '{}') ?? {}
    return {
      theme: value.theme === 'auto' ? 'tos' : ['tos', 'browser', 'light', 'dark'].includes(value.theme) ? value.theme : 'tos',
      language: value.language === 'auto' ? 'browser' : ['tos', 'browser', 'zh-CN', 'en-US'].includes(value.language) ? value.language : 'tos',
    }
  } catch { return { theme: 'tos', language: 'tos' } }
}

// This optional platform request must not affect application authentication or diagnostics.
async function readPlatformPreference<T>(path: string, parse: (payload: unknown) => T | null, signal: AbortSignal): Promise<T | null> {
  try {
    let csrf = ''
    try { csrf = document.cookie.split(';').map(value => value.trim()).find(value => value.startsWith('X-Csrf-Token='))?.slice(13) ?? '' } catch { /* Browser fallback still works. */ }
    const response = await fetch(path, {
      credentials: 'include', cache: 'no-store', redirect: 'error', signal,
      headers: csrf ? { 'X-Csrf-Token': csrf } : {},
    })
    if (!response.ok) return null
    return parse(await response.json())
  } catch { return null }
}

export const readDesktopTheme = async (signal: AbortSignal): Promise<Theme | null> => readDesktopThemeCookie() ?? readPlatformPreference('/v2/person/desktopDisplay', parseDesktopTheme, signal)
export const readDesktopLanguage = (signal: AbortSignal) => readPlatformPreference('/v2/timesetting/getlang', parseDesktopLanguage, signal)

export function initializePreferences(): () => void {
  let saved: string | null = null
  try { saved = localStorage.getItem(storageKey) } catch { /* Private/embedded contexts may block storage. */ }
  const initial = parsePreferences(saved)
  themePreference.value = initial.theme
  languagePreference.value = initial.language
  desktopTheme.value = themePreference.value === 'tos' ? readDesktopThemeCookie() : null
  desktopLanguage.value = null
  browserLanguage.value = browserLocale(navigator.languages ?? [navigator.language])
  const media = matchMedia('(prefers-color-scheme: dark)')
  browserDark.value = media.matches
  const requests = new Map<'theme' | 'language', AbortController>()
  let disposed = false

  const applyLanguage = () => {
    locale.value = language.value
    document.documentElement.lang = locale.value
    document.title = t('批量重命名')
  }
  const applyTheme = () => {
    document.documentElement.dataset.theme = theme.value
    document.documentElement.style.colorScheme = theme.value
  }
  const save = () => {
    try { localStorage.setItem(storageKey, JSON.stringify({ theme: themePreference.value, language: languagePreference.value })) } catch { /* Preferences remain usable for this session. */ }
  }
  const syncPreference = async <T>(key: 'theme' | 'language', read: (signal: AbortSignal) => Promise<T | null>, apply: (value: T | null) => void) => {
    if (disposed || requests.has(key) || document.visibilityState === 'hidden') return
    const controller = new AbortController()
    requests.set(key, controller)
    const timeout = setTimeout(() => controller.abort(), 2000)
    try {
      const value = await read(controller.signal)
      if (!disposed) apply(controller.signal.aborted ? null : value)
    } finally {
      clearTimeout(timeout)
      requests.delete(key)
    }
  }
  const syncDesktop = () => {
    if (themePreference.value === 'tos') void syncPreference('theme', readDesktopTheme, value => { desktopTheme.value = value })
    if (languagePreference.value === 'tos') void syncPreference('language', readDesktopLanguage, value => { desktopLanguage.value = value })
  }
  const onLanguageChange = () => { browserLanguage.value = browserLocale(navigator.languages ?? [navigator.language]) }
  const onMedia = (event: MediaQueryListEvent) => { browserDark.value = event.matches }
  const onStorage = (event: StorageEvent) => {
    if (event.key !== storageKey && event.key !== null) return
    const value = parsePreferences(event.newValue)
    themePreference.value = value.theme
    languagePreference.value = value.language
  }
  applyLanguage()
  applyTheme()
  const stops = [
    watch(language, applyLanguage, { flush: 'sync' }),
    watch(languagePreference, () => { save(); syncDesktop() }, { flush: 'sync' }),
    watch(theme, applyTheme, { flush: 'sync' }),
    watch(themePreference, () => { save(); void syncDesktop() }),
  ]
  media.addEventListener('change', onMedia)
  window.addEventListener('languagechange', onLanguageChange)
  window.addEventListener('focus', syncDesktop)
  window.addEventListener('storage', onStorage)
  document.addEventListener('visibilitychange', syncDesktop)
  const interval = setInterval(syncDesktop, 30000)
  void syncDesktop()
  return () => {
    disposed = true
    requests.forEach(request => request.abort())
    clearInterval(interval)
    stops.forEach(stop => stop())
    media.removeEventListener('change', onMedia)
    window.removeEventListener('languagechange', onLanguageChange)
    window.removeEventListener('focus', syncDesktop)
    window.removeEventListener('storage', onStorage)
    document.removeEventListener('visibilitychange', syncDesktop)
  }
}
