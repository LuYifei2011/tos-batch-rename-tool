import { afterEach, beforeEach, test } from 'node:test'
import assert from 'node:assert/strict'
import { locale, localizeMessage, t } from './i18n.ts'

const originalStorage = Object.getOwnPropertyDescriptor(globalThis, 'localStorage')
beforeEach(() => {
  Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: { setItem() {} } })
})
afterEach(() => {
  locale.value = 'zh-CN'
  if (originalStorage) Object.defineProperty(globalThis, 'localStorage', originalStorage)
  else Reflect.deleteProperty(globalThis, 'localStorage')
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
