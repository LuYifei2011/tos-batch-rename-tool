import { afterEach, beforeEach, test } from 'node:test'
import assert from 'node:assert/strict'
import { api, backendVersion, connectionDiagnostic, denied } from './api.ts'

const originalFetch = globalThis.fetch
const originalDocument = Object.getOwnPropertyDescriptor(globalThis, 'document')
const originalLocation = Object.getOwnPropertyDescriptor(globalThis, 'location')
let redirected = ''

beforeEach(() => {
  denied.value = ''
  backendVersion.value = ''
  connectionDiagnostic.value = null
  redirected = ''
  Object.defineProperty(globalThis, 'document', { configurable: true, value: { cookie: 'TMSESSNAME=test-session; X-Csrf-Token=test-csrf' } })
  Object.defineProperty(globalThis, 'location', { configurable: true, value: { pathname: '/batch-rename/', assign: (url: string) => { redirected = url } } })
})
afterEach(() => {
  globalThis.fetch = originalFetch
  for (const [key, descriptor] of [['document', originalDocument], ['location', originalLocation]] as const) {
    if (descriptor) Object.defineProperty(globalThis, key, descriptor)
    else Reflect.deleteProperty(globalThis, key)
  }
  denied.value = ''
})

test('API uses the platform proxy for desktop, alternate WebView entry paths and local preview', async () => {
  for (const pathname of ['/batch-rename/', '/batch-rename', '/webui/batch-rename/index.html', '/mobile/index.html', '/']) {
    Object.defineProperty(globalThis, 'location', { configurable: true, value: { pathname } })
    let requests = 0
    globalThis.fetch = async (input, options) => {
      requests++
      assert.equal(String(input), '/v2/proxy/batch-rename/api/info')
      assert.equal(options?.credentials, 'include')
      assert.equal(new Headers(options?.headers).get('X-Csrf-Token'), 'test-csrf')
      return Response.json({ mode: 'tos', roots: [] })
    }
    assert.deepEqual(await api('/info'), { mode: 'tos', roots: [] })
    assert.equal(requests, 1)
  }
})

test('source rejection keeps its diagnostic and blocks further requests', async () => {
  let requests = 0
  globalThis.fetch = async () => {
    requests++
    return Response.json({ code: 'ORIGIN_MISMATCH', error: '连接来源校验失败（ORIGIN_MISMATCH），请重新打开应用。' }, { status: 403 })
  }
  await assert.rejects(api('/info'), /ORIGIN_MISMATCH/)
  assert.doesNotMatch(denied.value, /没有此应用的访问权限/)
  await assert.rejects(api('/jobs'), /ORIGIN_MISMATCH/)
  assert.equal(requests, 1)
})

test('unclassified gateway 403 does not invent an account permission diagnosis or render HTML', async () => {
  globalThis.fetch = async () => new Response('<html>forbidden</html>', { status: 403 })
  await assert.rejects(api('/info'), /TOS 拒绝了应用请求（403）/)
  assert.doesNotMatch(denied.value, /联系管理员|<html>/)
})

test('explicit application permission denial is preserved', async () => {
  globalThis.fetch = async () => Response.json({ error: '当前账号没有此应用的访问权限' }, { status: 403 })
  await assert.rejects(api('/info'), /当前账号没有此应用的访问权限/)
  assert.equal(denied.value, '当前账号没有此应用的访问权限')
})

test('expired session still hides the app and redirects to TOS login', async () => {
  globalThis.fetch = async () => new Response(null, { status: 401 })
  await assert.rejects(api('/info'), /会话已失效/)
  assert.equal(redirected, '/tos/')
  assert.ok(denied.value)
})

test('mutating requests are never retried after a gateway failure', async () => {
  let requests = 0
  globalThis.fetch = async (input, options) => {
    requests++
    assert.equal(String(input), '/v2/proxy/batch-rename/api/execute')
    assert.equal(options?.method, 'POST')
    assert.equal(options?.body, JSON.stringify({ plan: 'test-plan' }))
    return new Response(null, { status: 503 })
  }
  await assert.rejects(api('/execute', { plan: 'test-plan' }), /503/)
  assert.equal(requests, 1)
})

test('a non-JSON success response is reported as an invalid service response', async () => {
  globalThis.fetch = async () => new Response('<html>unexpected portal</html>')
  await assert.rejects(api('/info'), /数据格式无效/)
})

test('TOS in-band session failure never reaches application data', async () => {
  globalThis.fetch = async () => Response.json({ is_login: false, code: false, code_num: 117, msg: 'please login' })
  await assert.rejects(api('/info'), /会话已失效/)
  assert.equal(redirected, '/tos/')
  assert.equal(connectionDiagnostic.value?.status, 200)
  assert.equal(connectionDiagnostic.value?.failure, 'platform-session')
  assert.equal(connectionDiagnostic.value?.responseCode, '117')
})

test('empty gateway rejection exposes a sanitized report and no invented backend version', async () => {
  Object.defineProperty(globalThis, 'location', { configurable: true, value: {
    origin: 'http://nas:8181', pathname: '/batch-rename/', search: '?token=secret-query&source=mobile',
  } })
  globalThis.fetch = async () => new Response(null, { status: 403 })
  await assert.rejects(api('/list?root=secret-root&path=secret-path'), /403/)
  const report = connectionDiagnostic.value!
  assert.equal(report.clientVersion, 'dev')
  assert.equal(report.serverVersion, null)
  assert.equal(report.responseFormat, 'empty')
  assert.equal(report.responseBytes, 0)
  assert.equal(report.csrfCookieAvailable, true)
  assert.equal(report.csrfHeaderConfigured, true)
  assert.equal(report.sessionCookieReadable, true)
  assert.equal(report.pageOrigin, 'http://nas:8181')
  assert.deepEqual(report.queryKeys, ['token', 'source'])
  assert.equal(report.requestPath, '/v2/proxy/batch-rename/api/list')
  assert.doesNotMatch(JSON.stringify(report), /test-session|test-csrf|secret-query|secret-root|secret-path/)
})

test('unreadable CSRF does not set an empty header or bypass gateway rejection', async () => {
  for (const blocked of [false, true]) {
    denied.value = ''
    Object.defineProperty(globalThis, 'document', { configurable: true, value: blocked
      ? { get cookie() { throw new Error('blocked') } } : { cookie: '' } })
    globalThis.fetch = async (_, options) => {
      assert.equal(new Headers(options?.headers).has('X-Csrf-Token'), false)
      assert.equal(options?.credentials, 'include')
      return new Response(null, { status: 403 })
    }
    await assert.rejects(api('/info'), /403/)
    assert.equal(connectionDiagnostic.value?.cookieAccess, blocked ? 'blocked' : 'readable')
    assert.equal(connectionDiagnostic.value?.csrfHeaderConfigured, false)
    assert.equal(connectionDiagnostic.value?.csrfCookieAvailable, false)
  }
})

test('backend diagnostics use whitelisted codes and mask job identifiers', async () => {
  globalThis.fetch = async () => Response.json({ code: 'ORIGIN_MISMATCH', extra: 'secret-response' }, {
    status: 403, headers: { 'X-Batch-Rename-Version': '9.9.9' },
  })
  await assert.rejects(api('/jobs/0123456789abcdef0123456789abcdef?summary=1'), /403/)
  assert.equal(backendVersion.value, '9.9.9')
  assert.equal(connectionDiagnostic.value?.serverVersion, '9.9.9')
  assert.equal(connectionDiagnostic.value?.responseCode, 'ORIGIN_MISMATCH')
  assert.equal(connectionDiagnostic.value?.requestPath, '/v2/proxy/batch-rename/api/jobs/:id')
  assert.doesNotMatch(JSON.stringify(connectionDiagnostic.value), /secret-response/)
})

test('network exceptions record request flags without serializing exception secrets', async () => {
  globalThis.fetch = async () => { throw new Error('secret-network-detail') }
  await assert.rejects(api('/info'), /网络请求失败/)
  assert.equal(connectionDiagnostic.value?.status, null)
  assert.equal(connectionDiagnostic.value?.responseFormat, 'unavailable')
  assert.equal(connectionDiagnostic.value?.failure, 'network')
  assert.doesNotMatch(JSON.stringify(connectionDiagnostic.value), /secret-network-detail/)
})

test('a concurrent success cannot erase an access denial report', async () => {
  let finish!: (value: Response) => void
  globalThis.fetch = async (input) => String(input).endsWith('/jobs')
    ? new Promise(resolve => { finish = resolve }) : new Response(null, { status: 403 })
  const pending = api('/jobs')
  await assert.rejects(api('/info'), /403/)
  finish(Response.json([]))
  await pending
  assert.equal(connectionDiagnostic.value?.status, 403)
  assert.equal(connectionDiagnostic.value?.requestPath, '/v2/proxy/batch-rename/api/info')
})
