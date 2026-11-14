import { computed } from 'vue'
import { TosApiError } from 'tos-app-core'
import { tos, tosApp } from './app.ts'

export type { ConnectionDiagnostic } from 'tos-app-core'
export const backendVersion = tos.backendVersion
export const connectionDiagnostic = tos.connectionDiagnostic
export const denied = computed(() => tos.accessFailure.value ? apiErrorMessage(tos.accessFailure.value) : '')

function apiErrorMessage(error: TosApiError): string {
  switch (error.kind) {
    case 'network': case 'timeout': return '网络请求失败，请查看连接诊断。'
    case 'response-read': return '响应读取失败，请查看连接诊断。'
    case 'session': return '会话已失效，请重新登录 TOS'
    case 'denied': case 'origin': return error.serverMessage ?? (error.status === 403 ? 'TOS 拒绝了应用请求（403），请查看下方连接诊断。' : 'TOS 未通过应用授权，请查看下方连接诊断。')
    case 'http': return error.serverMessage ?? `请求失败（${error.status}），请查看连接诊断。`
    case 'invalid-response': return '服务返回的数据格式无效，请查看连接诊断。'
  }
}
// Preserve the application's GET/POST shorthand while the shared client supports explicit methods.
export async function api<T>(path: string, body?: unknown): Promise<T> {
  try { return await tosApp.api.request<T>(path, { body }) } catch (error) {
    if (error instanceof TosApiError) throw new Error(apiErrorMessage(error))
    throw error
  }
}
