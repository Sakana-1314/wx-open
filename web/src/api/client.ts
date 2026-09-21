// 类型化 API 客户端：契约来自 openapi.yaml 生成的 schema.d.ts，全链路无 any。
import createClient from 'openapi-fetch'

import { useAuth } from '@/stores/auth'

import { API_BASE } from './config'
import type { paths } from './schema'

export { API_BASE }

export const client = createClient<paths>({
  baseUrl: API_BASE,
})

// 鉴权中间件：请求注入 Bearer 令牌；401 响应清除本地会话并回到登录页。
client.use({
  async onRequest({ request }) {
    const { token } = useAuth()
    if (token.value) {
      request.headers.set('Authorization', `Bearer ${token.value}`)
    }
    return request
  },
  async onResponse({ response }) {
    if (response.status === 401 && !window.location.pathname.startsWith('/login')) {
      const { clearAuth } = useAuth()
      clearAuth()
      window.location.href = '/login'
      return response
    }
    return response
  },
})

/** 响应错误结构（契约 Error schema）。 */
export interface ApiError {
  code: string
  message: string
  errcode?: number | null
}

/**
 * 从 openapi-fetch 错误对象安全提取消息。
 * 后端错误统一为 { code, message }，message 中已含微信 errmsg 与处置建议。
 */
export function errorMessage(err: unknown): string {
  if (err && typeof err === 'object' && 'message' in err && typeof err.message === 'string') {
    return err.message
  }
  return '请求失败，请稍后重试'
}

/** 从错误对象提取微信返回码（若存在）。 */
export function errorErrcode(err: unknown): number | null {
  if (err && typeof err === 'object' && 'errcode' in err) {
    const code = (err as { errcode?: unknown }).errcode
    if (typeof code === 'number') {
      return code
    }
  }
  return null
}
