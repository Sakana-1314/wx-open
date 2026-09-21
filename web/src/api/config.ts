// API 地址解析：构建期注入的后端 HOST 决定前端请求的 base url。
// 兼容三种取值：
//   1. 未配置（开发/同源部署）→ 回退相对 '/api/v1'（开发经 Vite proxy、生产经 nginx 反代）
//   2. 裸域名（https://host） → 补 '/api/v1'
//   3. 已含路径（https://host/api/v1）→ 直接使用

/** 去掉尾部斜杠（保留根 '/'）。 */
export function normalizeBaseUrl(value: string): string {
  const base = value.trim()
  if (base === '/') {
    return base
  }
  return base.replace(/\/+$/, '')
}

/** 解析 API base：返回形如 'https://host/api/v1' 或相对 '/api/v1'。 */
export function resolveApiBaseUrl(hostValue: string | undefined): string {
  const raw = hostValue?.trim()
  if (!raw) {
    return '/api/v1'
  }
  // 相对形式（如 '/api/v1'）直接使用。
  if (raw.startsWith('/')) {
    return normalizeBaseUrl(raw)
  }
  // 绝对 URL。
  let parsed: URL
  try {
    parsed = new URL(raw)
  } catch {
    return '/api/v1'
  }
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    return '/api/v1'
  }
  // 裸域名（pathname 为空或 '/'）时补 /api/v1。
  if (parsed.pathname === '' || parsed.pathname === '/') {
    return `${parsed.origin}/api/v1`
  }
  // 已含路径（如 https://host/api/v1）则按给定使用（去掉尾斜杠）。
  return `${parsed.origin}${normalizeBaseUrl(parsed.pathname)}`
}

/** 构建期注入的后端 HOST（未配置时为空串）。 */
export const API_HOST: string =
  typeof __API_HOST__ !== 'undefined' && __API_HOST__ ? __API_HOST__ : ''

/** 前端 API base url：已含 /api/v1，供 openapi-fetch 与资源 URL 使用。 */
export const API_BASE: string = resolveApiBaseUrl(API_HOST)