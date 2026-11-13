import { ref } from 'vue'
import { APP_VERSION } from './build.ts'

export const denied = ref('')
export const backendVersion = ref('')
export interface ConnectionDiagnostic {
  clientVersion: string
  serverVersion: string | null
  pageOrigin: string
  pagePath: string
  queryKeys: string[]
  requestPath: string
  method: string
  credentials: 'include'
  cookieAccess: 'readable' | 'blocked'
  sessionCookieReadable: boolean
  csrfCookieAvailable: boolean
  csrfHeaderConfigured: boolean
  status: number | null
  responseFormat: 'empty' | 'json' | 'html' | 'text' | 'unavailable'
  responseBytes: number
  responseCode: string | null
  failure: 'http' | 'network' | 'invalid-response' | 'platform-session' | 'platform-denied'
}
export const connectionDiagnostic = ref<ConnectionDiagnostic | null>(null)
const base = '/v2/proxy/batch-rename/api'

function errorMessage(data: unknown, fallback: string): string {
  if (data && typeof data === 'object' && 'error' in data && typeof data.error === 'string' && data.error.trim()) return data.error
  return fallback
}
function object(data: unknown): Record<string, unknown> {
  return data && typeof data === 'object' ? data as Record<string, unknown> : {}
}

export async function api<T>(path: string, body?: unknown): Promise<T> {
  if (denied.value) throw new Error(denied.value)
  let cookies = ''
  let cookieAccess: ConnectionDiagnostic['cookieAccess'] = 'readable'
  try { cookies = document.cookie } catch { cookieAccess = 'blocked' }
  const parts = cookies.split(';').map(x => x.trim())
  const csrf = parts.find(x => x.startsWith('X-Csrf-Token='))?.slice(13) ?? ''
  const method = body === undefined ? 'GET' : 'POST'
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  // An empty header would also override credentials supplied by a native container.
  if (csrf) headers['X-Csrf-Token'] = csrf
  const options: RequestInit = { method, credentials: 'include', headers, body: body === undefined ? undefined : JSON.stringify(body) }
  const report = (failure: ConnectionDiagnostic['failure'], response?: Response, text = '', data?: unknown) => {
    const payload = object(data)
    const code = payload.code ?? payload.code_num
    const safeCode = typeof code === 'number' ? String(code) : typeof code === 'string' && /^[A-Z][A-Z0-9_-]{0,63}$/.test(code) ? code : typeof payload.code_num === 'number' ? String(payload.code_num) : null
    const version = response?.headers.get('X-Batch-Rename-Version') ?? ''
    connectionDiagnostic.value = {
      clientVersion: APP_VERSION,
      serverVersion: /^[\w.+-]{1,64}$/.test(version) ? version : null,
      pageOrigin: location.origin ?? '', pagePath: location.pathname ?? '',
      queryKeys: [...new URLSearchParams(location.search ?? '').keys()].filter(x => /^[a-z][a-z0-9_-]{0,40}$/i.test(x)).slice(0,20),
      requestPath: (base + path.split('?')[0]).replace(/\/jobs\/[a-f0-9]{32}/, '/jobs/:id'), method, credentials: 'include',
      cookieAccess, sessionCookieReadable: parts.some(x => /^TMSESSNAME=.+/.test(x)),
      csrfCookieAvailable: !!csrf, csrfHeaderConfigured: !!headers['X-Csrf-Token'],
      status: response?.status ?? null,
      responseFormat: !response ? 'unavailable' : !text.trim() ? 'empty' : data !== undefined ? 'json' : text.trimStart().startsWith('<') ? 'html' : 'text',
      responseBytes: new TextEncoder().encode(text).length, responseCode: safeCode, failure,
    }
  }
  let r: Response
  try {
    r = await fetch(base + path, options)
    for (let attempt = 0; body === undefined && !denied.value && [502, 503, 504].includes(r.status) && attempt < 3; attempt++) {
      await r.body?.cancel()
      await new Promise(resolve => setTimeout(resolve, 500 * 2 ** attempt))
      r = await fetch(base + path, options)
    }
  } catch {
    report('network')
    throw new Error('网络请求失败，请查看连接诊断。')
  }
  const version = r.headers.get('X-Batch-Rename-Version') ?? ''
  if (/^[\w.+-]{1,64}$/.test(version)) backendVersion.value = version
  let text: string
  try { text = await r.text() } catch {
    report('network', r)
    throw new Error('响应读取失败，请查看连接诊断。')
  }
  let data: unknown
  try { data = JSON.parse(text) } catch { /* A gateway can return an empty or HTML error. */ }
  if (r.status === 401 || object(data).is_login === false) {
    report(object(data).is_login === false ? 'platform-session' : 'http', r, text, data)
    denied.value = '会话已失效，请重新登录 TOS'
    location.assign('/tos/')
    throw new Error(denied.value)
  }
  if (r.status === 403 || (object(data).is_login === true && object(data).code === false)) {
    report(r.status === 403 ? 'http' : 'platform-denied', r, text, data)
    denied.value = errorMessage(data, r.status === 403 ? 'TOS 拒绝了应用请求（403），请查看下方连接诊断。' : 'TOS 未通过应用授权，请查看下方连接诊断。')
    throw new Error(denied.value)
  }
  if (!r.ok) {
    report('http', r, text, data)
    throw new Error(errorMessage(data, `请求失败（${r.status}），请查看连接诊断。`))
  }
  if (data === undefined || data === null) {
    report('invalid-response', r, text, data)
    throw new Error('服务返回的数据格式无效，请查看连接诊断。')
  }
  if (!denied.value) connectionDiagnostic.value = null
  return data as T
}
