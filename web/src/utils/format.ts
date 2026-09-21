// 展示层纯函数集合：时间戳格式化 + 业务状态文案/颜色映射。
// 约定：
//   1. 全部为无副作用纯函数，便于在任意 view 复用并单测；
//   2. 空值（null / undefined / 空串）统一返回占位符 '—'；
//   3. 时间戳解析失败时返回原值，绝不抛异常。
// 后端返回的时间均为 RFC3339 字符串（如 2025-01-02T03:04:05+08:00）或 null。

/** 数据缺失时统一展示的占位符。 */
export const EMPTY_TEXT = '—'

/** 与 NaiveUI `n-tag` / `n-alert` 的 type 取值保持一致。 */
export type TagType = 'default' | 'info' | 'success' | 'warning' | 'error'

/** 状态语义域：决定 status 原值如何映射为文案与颜色。 */
export type StatusKind = 'job' | 'item' | 'audit' | 'boolean' | 'authorizer'

/** 两位补零。 */
function pad2(value: number): string {
  return String(value).padStart(2, '0')
}

/** 判断是否为空值（null / undefined / 纯空白），同时作为类型守卫收窄为 string。 */
function isBlank(value: string | null | undefined): value is null | undefined {
  return value === null || value === undefined || value.trim() === ''
}

/**
 * RFC3339 时间戳 → 本地时区 'YYYY-MM-DD HH:mm:ss'。
 * 空值返回 '—'；解析失败返回原字符串（保留后端原始信息便于排障）。
 */
export function formatDateTime(value: string | null | undefined): string {
  if (isBlank(value)) {
    return EMPTY_TEXT
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  const day = `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`
  const time = `${pad2(date.getHours())}:${pad2(date.getMinutes())}:${pad2(date.getSeconds())}`
  return `${day} ${time}`
}

/** 时间戳 → 'YYYY-MM-DD'（空值 '—'，解析失败返回原值）。 */
export function formatDate(value: string | null | undefined): string {
  if (isBlank(value)) {
    return EMPTY_TEXT
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`
}

const MINUTE_MS = 60_000
const HOUR_MS = 60 * MINUTE_MS
const DAY_MS = 24 * HOUR_MS
/** 超过该跨度后不再用相对时间，直接展示绝对时间。 */
const RELATIVE_LIMIT_MS = 30 * DAY_MS

/**
 * 相对当前时间的中文描述：刚刚 / N 分钟前 / N 小时前 / N 天前（未来时间用「后」）。
 * 空值返回 '—'，解析失败返回原值，超过 30 天回退为绝对时间。
 */
export function formatRelativeTime(value: string | null | undefined): string {
  if (isBlank(value)) {
    return EMPTY_TEXT
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  const diff = date.getTime() - Date.now()
  const abs = Math.abs(diff)
  if (abs < MINUTE_MS) {
    return '刚刚'
  }
  const suffix = diff >= 0 ? '后' : '前'
  if (abs < HOUR_MS) {
    return `${Math.floor(abs / MINUTE_MS)} 分钟${suffix}`
  }
  if (abs < DAY_MS) {
    return `${Math.floor(abs / HOUR_MS)} 小时${suffix}`
  }
  if (abs < RELATIVE_LIMIT_MS) {
    return `${Math.floor(abs / DAY_MS)} 天${suffix}`
  }
  return formatDateTime(value)
}

/** 微信审核状态码 → 中文文案（0 成功 / 1 拒绝 / 2 审核中 / 3 撤回 / 4 延后）。 */
export function auditStatusText(status: number): string {
  switch (status) {
    case 0:
      return '审核成功'
    case 1:
      return '审核被拒绝'
    case 2:
      return '审核中'
    case 3:
      return '已撤回'
    case 4:
      return '审核延后'
    default:
      return '未知状态'
  }
}

/** 微信审核状态码 → 标签颜色。 */
export function auditStatusType(status: number): TagType {
  switch (status) {
    case 0:
      return 'success'
    case 1:
      return 'error'
    case 2:
      return 'info'
    case 3:
      return 'default'
    case 4:
      return 'warning'
    default:
      return 'default'
  }
}

/** 作业状态（JobStatus）→ 中文文案。 */
export function jobStatusText(status: string): string {
  switch (status) {
    case 'pending':
      return '待执行'
    case 'running':
      return '运行中'
    case 'paused':
      return '已暂停'
    case 'succeeded':
      return '已成功'
    case 'partial_failed':
      return '部分失败'
    case 'failed':
      return '已失败'
    case 'canceled':
      return '已取消'
    case 'interrupted':
      return '已中断'
    default:
      return '未知状态'
  }
}

/** 作业状态 → 标签颜色。 */
export function jobStatusType(status: string): TagType {
  switch (status) {
    case 'pending':
      return 'default'
    case 'running':
      return 'info'
    case 'succeeded':
      return 'success'
    case 'partial_failed':
      return 'warning'
    case 'paused':
      return 'warning'
    case 'failed':
      return 'error'
    case 'interrupted':
      return 'error'
    case 'canceled':
      return 'default'
    default:
      return 'default'
  }
}

/** 作业类型（JobType）→ 中文文案。 */
export function jobTypeText(type: string): string {
  switch (type) {
    case 'commit':
      return '上传代码'
    case 'submit_audit':
      return '提交审核'
    case 'release':
      return '发布'
    case 'pipeline':
      return '全流程'
    case 'sync_info':
      return '同步小程序信息'
    case 'sync_audit_status':
      return '同步审核状态'
    case 'undo_audit':
      return '撤回审核'
    case 'speedup_audit':
      return '加急审核'
    case 'set_domain':
      return '配置域名'
    case 'revert':
      return '版本回退'
    case 'toggle_visit':
      return '服务状态'
    default:
      return '未知类型'
  }
}

/** 作业条目状态（JobItemStatus）→ 中文文案。 */
export function itemStatusText(status: string): string {
  switch (status) {
    case 'pending':
      return '待执行'
    case 'waiting':
      return '等待前置'
    case 'running':
      return '执行中'
    case 'succeeded':
      return '成功'
    case 'failed':
      return '失败'
    case 'skipped':
      return '已跳过'
    case 'canceled':
      return '已取消'
    default:
      return '未知状态'
  }
}

/** 作业条目状态 → 标签颜色。 */
export function itemStatusType(status: string): TagType {
  switch (status) {
    case 'pending':
      return 'default'
    case 'waiting':
      return 'warning'
    case 'running':
      return 'info'
    case 'succeeded':
      return 'success'
    case 'failed':
      return 'error'
    case 'skipped':
      return 'default'
    case 'canceled':
      return 'default'
    default:
      return 'default'
  }
}

/** 授权状态（AuthorizationStatus）→ 中文文案。 */
export function authorizationStatusText(status: string): string {
  switch (status) {
    case 'authorized':
      return '已授权'
    case 'unauthorized':
      return '已取消授权'
    default:
      return '未知状态'
  }
}

/**
 * 额度文案：rest 为 null/undefined 时视为「未探测」（服务商额度需调用微信接口后才能得到）。
 * limit 存在时展示 'rest / limit'。
 */
export function formatQuota(
  rest: number | null | undefined,
  limit?: number | null | undefined,
): string {
  if (rest === null || rest === undefined) {
    return '未探测'
  }
  if (limit === null || limit === undefined) {
    return String(rest)
  }
  return `${rest} / ${limit}`
}

/** 用量百分比：limit 缺失或非正数时返回 0，结果夹在 0-100。 */
export function usagePercent(used: number | null | undefined, limit: number | null | undefined): number {
  if (used === null || used === undefined || limit === null || limit === undefined || limit <= 0) {
    return 0
  }
  const percent = Math.round((used / limit) * 100)
  return Math.min(100, Math.max(0, percent))
}

/**
 * 中间截断长字符串（appid / 作业 id 等），保留首尾便于人工比对。
 * 长度不足以截断时原样返回。
 */
export function truncateMiddle(value: string, head = 8, tail = 6): string {
  const safeHead = Math.max(0, head)
  const safeTail = Math.max(0, tail)
  if (value.length <= safeHead + safeTail + 1) {
    return value
  }
  return `${value.slice(0, safeHead)}…${value.slice(value.length - safeTail)}`
}

/** 布尔状态 → 标签颜色（true 绿 / false 红），用于票据、令牌等可用性判断。 */
export function booleanStatusType(value: boolean): TagType {
  return value ? 'success' : 'error'
}
