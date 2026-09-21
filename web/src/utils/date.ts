// 断言某值非空；避免 any 的环境下安全解包可选值。
export function nonNull<T>(value: T | null | undefined, message = '数据缺失'): T {
  if (value === null || value === undefined) {
    throw new Error(message)
  }
  return value
}

/** 今天（YYYY-MM-DD）。 */
export function today(): string {
  return formatDate(new Date())
}

/** 日期对象转 YYYY-MM-DD。 */
export function formatDate(d: Date): string {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

/** 解析 YYYY-MM-DD 为 Date（本地时区）。 */
export function parseDate(s: string): Date {
  const [y, m, d] = s.split('-').map(Number)
  return new Date((y ?? 1970), (m ?? 1) - 1, d ?? 1)
}

/** YYYY-MM-DD 加 n 天。 */
export function addDays(dateStr: string, days: number): string {
  const d = parseDate(dateStr)
  d.setDate(d.getDate() + days)
  return formatDate(d)
}

/** 判断是否逾期（dueDate 早于今天）。 */
export function isOverdue(dueDate: string, status: string): boolean {
  if (status === '已整改') {
    return false
  }
  return dueDate < today()
}