import { afterEach, beforeEach, test } from 'node:test'
import assert from 'node:assert/strict'
import { nextTick } from 'vue'
import { browserLocale, initializePreferences, languagePreference, languageSource, parseDesktopLanguage, parseDesktopTheme, parsePreferences, readDesktopLanguage, readDesktopTheme, readDesktopThemeCookie, theme, themePreference, themeSource } from './preferences.ts'
import { locale, localizeMessage, t } from './i18n.ts'

const keys = ['fetch', 'document', 'window', 'navigator', 'localStorage', 'matchMedia'] as const
const originals = new Map(keys.map(key => [key, Object.getOwnPropertyDescriptor(globalThis, key)]))
let dispose: (() => void) | undefined
let storage: Map<string, string>
let media: EventTarget & { matches: boolean }
let page: EventTarget & { cookie: string; visibilityState: string; title: string; documentElement: { lang: string; dataset: Record<string, string>; style: Record<string, string> } }
const flush = async () => { await nextTick(); await new Promise(resolve => setImmediate(resolve)) }

beforeEach(() => {
  storage = new Map()
  media = Object.assign(new EventTarget(), { matches: false })
  page = Object.assign(new EventTarget(), { cookie: 'X-Csrf-Token=test-csrf', visibilityState: 'visible', title: '', documentElement: { lang: '', dataset: {}, style: {} } })
  const mocks = {
    document: page, window: new EventTarget(), navigator: { languages: ['zh-CN'] },
    localStorage: { getItem: (key: string) => storage.get(key) ?? null, setItem: (key: string, value: string) => storage.set(key, value) },
    matchMedia: () => media, fetch: async () => new Response(null, { status: 404 }),
  }
  for (const key of keys) Object.defineProperty(globalThis, key, { configurable: true, writable: true, value: mocks[key] })
})
afterEach(() => {
  dispose?.()
  dispose = undefined
  for (const key of keys) {
    const descriptor = originals.get(key)
    if (descriptor) Object.defineProperty(globalThis, key, descriptor)
    else Reflect.deleteProperty(globalThis, key)
  }
  locale.value = 'zh-CN'
})

test('browser language negotiation and stored preference validation', () => {
  assert.equal(browserLocale(['de-DE', 'en-GB', 'zh-CN']), 'en-US')
  assert.equal(browserLocale(['zh-TW', 'en-US']), 'zh-CN')
  assert.equal(browserLocale(['fr-FR']), 'en-US')
  for (const raw of [null, '{', 'null', 'true', '[]', '{"theme":"purple","language":"fr"}']) assert.deepEqual(parsePreferences(raw), { theme: 'tos', language: 'tos' })
  assert.deepEqual(parsePreferences('{"theme":"light","language":"en-US"}'), { theme: 'light', language: 'en-US' })
  assert.deepEqual(parsePreferences('{"theme":"auto","language":"auto"}'), { theme: 'tos', language: 'browser' })
  assert.deepEqual(parsePreferences('{"theme":"browser","language":"tos"}'), { theme: 'browser', language: 'tos' })
})

test('only successful TOS envelopes with recognized modes select a desktop theme', () => {
  assert.equal(parseDesktopTheme({ is_login: true, code: true, data: { display_mode: 'dark', color: '#3491FA' } }), 'dark')
  assert.equal(parseDesktopTheme({ code: true, data: { display_mode: 'light' } }), 'light')
  for (const payload of [null, {}, { code: false, data: { display_mode: 'dark' } }, { code: true, is_login: false, data: { display_mode: 'dark' } }, { code: true, data: { display_mode: 'unknown' } }]) assert.equal(parseDesktopTheme(payload), null)
})

test('TOS language uses tos_lang and maps unsupported locales to English', () => {
  assert.equal(parseDesktopLanguage({ is_login: true, code: true, data: { tos_lang: 'zh-cn', notify_lang: 'en-us' } }), 'zh-CN')
  assert.equal(parseDesktopLanguage({ code: true, data: { tos_lang: 'zh-hk' } }), 'zh-CN')
  assert.equal(parseDesktopLanguage({ code: true, data: { tos_lang: 'en-us' } }), 'en-US')
  assert.equal(parseDesktopLanguage({ code: true, data: { tos_lang: 'de-de' } }), 'en-US')
  for (const payload of [null, {}, { code: false, data: { tos_lang: 'zh-cn' } }, { code: true, is_login: false, data: { tos_lang: 'zh-cn' } }, { code: true, data: { notify_lang: 'zh-cn' } }, { code: true, data: { tos_lang: '' } }, { code: true, data: { tos_lang: 1 } }]) assert.equal(parseDesktopLanguage(payload), null)
})

test('language request uses the same-origin TOS session and ignores rejected responses', async () => {
  globalThis.fetch = async (url, options) => {
    assert.equal(url, '/v2/timesetting/getlang')
    assert.equal(options?.credentials, 'include')
    assert.equal(options?.redirect, 'error')
    assert.equal(new Headers(options?.headers).get('X-Csrf-Token'), 'test-csrf')
    return Response.json({ is_login: true, code: true, data: { tos_lang: 'en-us' } })
  }
  assert.equal(await readDesktopLanguage(new AbortController().signal), 'en-US')
  globalThis.fetch = async () => new Response(null, { status: 403 })
  assert.equal(await readDesktopLanguage(new AbortController().signal), null)
})

test('TOS language and browser language can be selected independently of the theme', async () => {
  storage.set('batch-rename.preferences', JSON.stringify({ theme: 'browser', language: 'tos' }))
  let requests = 0
  let tosLanguage = 'en-us'
  globalThis.fetch = async url => {
    requests++
    assert.equal(url, '/v2/timesetting/getlang')
    return Response.json({ code: true, data: { tos_lang: tosLanguage } })
  }
  dispose = initializePreferences()
  await flush()
  assert.equal(locale.value, 'en-US')
  assert.equal(languageSource.value, 'tos')
  assert.equal(page.title, 'Batch Rename')
  assert.equal(theme.value, 'light')
  tosLanguage = 'zh-cn'
  window.dispatchEvent(new Event('focus'))
  await flush()
  assert.equal(locale.value, 'zh-CN')
  languagePreference.value = 'browser'
  await flush()
  assert.equal(languageSource.value, 'browser')
  const before = requests
  window.dispatchEvent(new Event('focus'))
  await flush()
  assert.equal(requests, before)
  Object.defineProperty(globalThis, 'navigator', { configurable: true, value: { languages: ['en-GB'] } })
  window.dispatchEvent(new Event('languagechange'))
  assert.equal(locale.value, 'en-US')
})

test('browser preferences do not request either TOS endpoint', async () => {
  storage.set('batch-rename.preferences', JSON.stringify({ theme: 'browser', language: 'browser' }))
  let requests = 0
  globalThis.fetch = async () => { requests++; return new Response(null, { status: 500 }) }
  dispose = initializePreferences()
  await flush()
  window.dispatchEvent(new Event('focus'))
  await flush()
  assert.equal(requests, 0)
  assert.equal(locale.value, 'zh-CN')
  assert.equal(theme.value, 'light')
})

test('a valid TOS theme cookie applies synchronously and avoids the display request', async () => {
  storage.set('batch-rename.preferences', JSON.stringify({ theme: 'tos', language: 'browser' }))
  page.cookie = 'X-Csrf-Token=test-csrf; themeMode=dark; themeColor=blue'
  let requests = 0
  globalThis.fetch = async () => { requests++; return new Response(null, { status: 500 }) }
  dispose = initializePreferences()
  assert.equal(page.documentElement.dataset.theme, 'dark')
  await flush()
  assert.equal(themeSource.value, 'tos')
  assert.equal(requests, 0)
  page.cookie = 'themeMode=light'
  window.dispatchEvent(new Event('focus'))
  await flush()
  assert.equal(theme.value, 'light')
  themePreference.value = 'browser'
  await flush()
  page.cookie = 'themeMode=dark'
  window.dispatchEvent(new Event('focus'))
  await flush()
  assert.equal(theme.value, 'light')
  assert.equal(themeSource.value, 'browser')
  assert.equal(requests, 0)
})

test('missing, malformed and inaccessible theme cookies fall back to the display API', async () => {
  let requests = 0
  globalThis.fetch = async url => {
    requests++
    assert.equal(url, '/v2/person/desktopDisplay')
    return Response.json({ code: true, data: { display_mode: 'light' } })
  }
  for (const cookie of ['', 'themeMode=unknown', 'other=themeMode=dark', 'themeMode=', 'themeMode=dark-extra']) {
    page.cookie = cookie
    assert.equal(readDesktopThemeCookie(), null)
    assert.equal(await readDesktopTheme(new AbortController().signal), 'light')
  }
  Object.defineProperty(page, 'cookie', { get() { throw new Error('blocked') } })
  assert.equal(await readDesktopTheme(new AbortController().signal), 'light')
  assert.equal(requests, 6)
})

test('a language response cannot override an explicit choice made while it was pending', async () => {
  storage.set('batch-rename.preferences', JSON.stringify({ theme: 'light', language: 'tos' }))
  let finish!: (response: Response) => void
  globalThis.fetch = async () => new Promise(resolve => { finish = resolve })
  dispose = initializePreferences()
  languagePreference.value = 'zh-CN'
  finish(Response.json({ code: true, data: { tos_lang: 'en-us' } }))
  await flush()
  assert.equal(locale.value, 'zh-CN')
  languagePreference.value = 'browser'
  assert.equal(locale.value, 'zh-CN')
})

test('display request stays same-origin with the existing CSRF session', async () => {
  globalThis.fetch = async (url, options) => {
    assert.equal(url, '/v2/person/desktopDisplay')
    assert.equal(options?.credentials, 'include')
    assert.equal(options?.redirect, 'error')
    assert.equal(options?.cache, 'no-store')
    assert.equal(new Headers(options?.headers).get('X-Csrf-Token'), 'test-csrf')
    return Response.json({ code: true, data: { display_mode: 'dark' } })
  }
  assert.equal(await readDesktopTheme(new AbortController().signal), 'dark')
})

test('display failures quietly fall back even when cookies are blocked', async () => {
  Object.defineProperty(page, 'cookie', { get() { throw new Error('blocked') } })
  globalThis.fetch = async (_, options) => {
    assert.equal(new Headers(options?.headers).has('X-Csrf-Token'), false)
    return new Response('<html>forbidden</html>', { status: 403 })
  }
  assert.equal(await readDesktopTheme(new AbortController().signal), null)
  globalThis.fetch = async () => new Response('<html>login</html>')
  assert.equal(await readDesktopTheme(new AbortController().signal), null)
  globalThis.fetch = async () => { throw new Error('offline') }
  dispose = initializePreferences()
  await flush()
  assert.equal(theme.value, 'light')
  assert.equal(themeSource.value, 'browser')
  assert.equal(page.documentElement.dataset.theme, 'light')
})

test('TOS theme wins in auto mode and explicit preferences survive reinitialization', async () => {
  globalThis.fetch = async () => Response.json({ code: true, data: { display_mode: 'dark' } })
  dispose = initializePreferences()
  await flush()
  assert.equal(theme.value, 'dark')
  assert.equal(themeSource.value, 'tos')
  themePreference.value = 'light'
  languagePreference.value = 'en-US'
  await flush()
  assert.equal(page.documentElement.dataset.theme, 'light')
  assert.equal(page.documentElement.lang, 'en-US')
  assert.equal(page.title, 'Batch Rename')
  assert.deepEqual(JSON.parse(storage.get('batch-rename.preferences')!), { theme: 'light', language: 'en-US' })
  dispose()
  dispose = initializePreferences()
  await flush()
  assert.equal(theme.value, 'light')
  assert.equal(locale.value, 'en-US')
})

test('browser appearance changes are applied when TOS is unavailable', async () => {
  dispose = initializePreferences()
  await flush()
  media.dispatchEvent(Object.assign(new Event('change'), { matches: true }))
  await flush()
  assert.equal(page.documentElement.dataset.theme, 'dark')
  themePreference.value = 'light'
  await flush()
  media.dispatchEvent(Object.assign(new Event('change'), { matches: true }))
  await flush()
  assert.equal(page.documentElement.dataset.theme, 'light')
})

test('storage denial does not prevent language or theme changes', async () => {
  Object.defineProperty(globalThis, 'localStorage', { configurable: true, get() { throw new Error('blocked') } })
  dispose = initializePreferences()
  await flush()
  languagePreference.value = 'en-US'
  themePreference.value = 'dark'
  await flush()
  assert.equal(page.title, 'Batch Rename')
  assert.equal(page.documentElement.dataset.theme, 'dark')
})

test('cleanup aborts a pending request and ignores its late result', async () => {
  const finishes: Array<(response: Response) => void> = []
  const signals: AbortSignal[] = []
  globalThis.fetch = async (_, options) => {
    if (options?.signal) signals.push(options.signal)
    return new Promise(resolve => { finishes.push(resolve) })
  }
  dispose = initializePreferences()
  dispose()
  dispose = undefined
  assert.equal(signals.length, 2)
  assert.ok(signals.every(signal => signal.aborted))
  const before = theme.value
  for (const finish of finishes) finish(Response.json({ code: true, data: { display_mode: 'dark', tos_lang: 'en-us' } }))
  await flush()
  assert.equal(theme.value, before)
  assert.equal(locale.value, 'zh-CN')
})

test('a timed-out refresh falls back from the previous TOS theme', async () => {
  globalThis.fetch = async () => Response.json({ code: true, data: { display_mode: 'dark' } })
  dispose = initializePreferences()
  await flush()
  assert.equal(theme.value, 'dark')
  globalThis.fetch = async (_, options) => new Promise((_, reject) => {
    options?.signal?.addEventListener('abort', () => reject(new Error('aborted')), { once: true })
  })
  window.dispatchEvent(new Event('focus'))
  await new Promise(resolve => setTimeout(resolve, 2100))
  assert.equal(theme.value, 'light')
  assert.equal(themeSource.value, 'browser')
})

test('a timed-out language refresh falls back from TOS to the browser language', async () => {
  storage.set('batch-rename.preferences', JSON.stringify({ theme: 'light', language: 'tos' }))
  globalThis.fetch = async () => Response.json({ code: true, data: { tos_lang: 'en-us' } })
  dispose = initializePreferences()
  await flush()
  assert.equal(locale.value, 'en-US')
  globalThis.fetch = async (_, options) => new Promise((_, reject) => {
    options?.signal?.addEventListener('abort', () => reject(new Error('aborted')), { once: true })
  })
  window.dispatchEvent(new Event('focus'))
  await new Promise(resolve => setTimeout(resolve, 2100))
  assert.equal(locale.value, 'zh-CN')
  assert.equal(languageSource.value, 'browser')
})

test('language changes translate errors without changing file names or template tokens', () => {
  locale.value = 'en-US'
  assert.equal(t('重命名 {count} 项', { count: '2,000' }), 'Rename 2,000 items')
  assert.equal(t('重命名 {count} 项', { count: 1 }), 'Rename 1 item')
  assert.equal(t('{count} 个共享文件夹', { count: '1' }), '1 shared folder')
  assert.equal(localizeMessage('目标名称冲突: 会议_{name}.txt'), 'Target name conflict: 会议_{name}.txt')
  assert.equal(localizeMessage('请求失败（503），请查看连接诊断。'), 'Request failed (503). See connection diagnostics.')
  assert.equal(localizeMessage('无法获取执行状态：网络请求失败，请查看连接诊断。'), 'Could not get task status: Network request failed. See connection diagnostics.')
  assert.equal(t('{name} 原名 · {n} 序号 · {date} 修改日期'), '{name} name · {n} number · {date} modified date')
  assert.equal(localizeMessage('unknown platform detail'), 'unknown platform detail')
  assert.equal(localizeMessage('constructor'), 'constructor')
  locale.value = 'zh-CN'
  assert.equal(localizeMessage('目标名称冲突: 会议_{name}.txt'), '目标名称冲突: 会议_{name}.txt')
})
