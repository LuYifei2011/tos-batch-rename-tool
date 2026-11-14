import { afterEach, beforeEach, test } from 'node:test'
import assert from 'node:assert/strict'
import { nextTick } from 'vue'
import { initializePreferences, languagePreference, theme, themePreference, themeSource } from './preferences.ts'
import { locale } from './i18n.ts'

const keys = ['fetch', 'document', 'window', 'navigator', 'localStorage', 'matchMedia'] as const
const originals = new Map(keys.map(key => [key, Object.getOwnPropertyDescriptor(globalThis, key)]))
let dispose: (() => void) | undefined
let storage: Map<string, string>
const flush = async () => { await nextTick(); await new Promise(resolve => setImmediate(resolve)) }

beforeEach(() => {
  storage = new Map()
  const media = Object.assign(new EventTarget(), { matches: false })
  const page = Object.assign(new EventTarget(), { cookie: 'X-Csrf-Token=test-csrf', visibilityState: 'visible', title: '', documentElement: { lang: '', dataset: {}, style: {} } })
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
  locale.value = 'zh-CN'
  for (const key of keys) {
    const descriptor = originals.get(key)
    if (descriptor) Object.defineProperty(globalThis, key, descriptor)
    else Reflect.deleteProperty(globalThis, key)
  }
})

test('application preferences use their namespace and keep Vue bindings across restart', async () => {
  globalThis.fetch = async () => Response.json({ code: true, data: { display_mode: 'dark' } })
  dispose = initializePreferences()
  await flush()
  assert.equal(theme.value, 'dark')
  assert.equal(themeSource.value, 'tos')
  themePreference.value = 'light'
  languagePreference.value = 'en-US'
  await flush()
  assert.deepEqual(JSON.parse(storage.get('batch-rename.preferences')!), { theme: 'light', language: 'en-US' })
  dispose()
  dispose = initializePreferences()
  await flush()
  assert.equal(theme.value, 'light')
  assert.equal(locale.value, 'en-US')
})
