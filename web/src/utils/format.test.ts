// utils/format.ts 单元测试：覆盖空值、解析失败、各状态枚举映射与截断边界。
import { describe, expect, it } from 'vitest'

import {
  auditStatusText,
  auditStatusType,
  authorizationStatusText,
  booleanStatusType,
  EMPTY_TEXT,
  formatDate,
  formatDateTime,
  formatQuota,
  formatRelativeTime,
  itemStatusText,
  itemStatusType,
  jobStatusText,
  jobStatusType,
  jobTypeText,
  truncateMiddle,
  usagePercent,
} from './format'

describe('formatDateTime', () => {
  it('空值统一返回占位符', () => {
    expect(formatDateTime(null)).toBe(EMPTY_TEXT)
    expect(formatDateTime(undefined)).toBe(EMPTY_TEXT)
    expect(formatDateTime('')).toBe(EMPTY_TEXT)
    expect(formatDateTime('   ')).toBe(EMPTY_TEXT)
  })

  it('解析失败时返回原值', () => {
    expect(formatDateTime('not-a-date')).toBe('not-a-date')
  })

  it('输出 YYYY-MM-DD HH:mm:ss 且与本地时区一致', () => {
    const iso = '2025-01-02T03:04:05.000Z'
    const out = formatDateTime(iso)
    expect(out).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/)
    // 用同一个 Date 对象按本地时区推导期望值，避免测试依赖运行环境的时区。
    const d = new Date(iso)
    const expected = [
      d.getFullYear(),
      String(d.getMonth() + 1).padStart(2, '0'),
      String(d.getDate()).padStart(2, '0'),
    ].join('-')
    expect(out.startsWith(expected)).toBe(true)
  })
})

describe('formatDate', () => {
  it('空值与非法值', () => {
    expect(formatDate(null)).toBe(EMPTY_TEXT)
    expect(formatDate('xx')).toBe('xx')
  })

  it('截取日期部分', () => {
    expect(formatDate('2025-06-07T08:09:10.000Z')).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  })
})

describe('formatRelativeTime', () => {
  it('空值返回占位符', () => {
    expect(formatRelativeTime(null)).toBe(EMPTY_TEXT)
    expect(formatRelativeTime(undefined)).toBe(EMPTY_TEXT)
    expect(formatRelativeTime('bad')).toBe('bad')
  })

  it('一分钟内视为刚刚', () => {
    expect(formatRelativeTime(new Date(Date.now() - 10_000).toISOString())).toBe('刚刚')
  })

  it('按分钟/小时/天给出「前」', () => {
    expect(formatRelativeTime(new Date(Date.now() - 90_000).toISOString())).toBe('1 分钟前')
    expect(formatRelativeTime(new Date(Date.now() - 2 * 3600_000).toISOString())).toBe('2 小时前')
    expect(formatRelativeTime(new Date(Date.now() - 3 * 86400_000).toISOString())).toBe('3 天前')
  })

  it('未来时间给出「后」', () => {
    expect(formatRelativeTime(new Date(Date.now() + 90_000).toISOString())).toBe('1 分钟后')
  })

  it('超过 30 天回退为绝对时间', () => {
    const iso = new Date(Date.now() - 60 * 86400_000).toISOString()
    expect(formatRelativeTime(iso)).toBe(formatDateTime(iso))
  })
})

describe('auditStatusText / auditStatusType', () => {
  it('已知状态码映射正确', () => {
    expect(auditStatusText(0)).toBe('审核成功')
    expect(auditStatusText(1)).toBe('审核被拒绝')
    expect(auditStatusText(2)).toBe('审核中')
    expect(auditStatusText(3)).toBe('已撤回')
    expect(auditStatusText(4)).toBe('审核延后')
  })

  it('未知状态码回退', () => {
    expect(auditStatusText(99)).toBe('未知状态')
    expect(auditStatusType(99)).toBe('default')
  })

  it('颜色映射', () => {
    expect(auditStatusType(0)).toBe('success')
    expect(auditStatusType(1)).toBe('error')
    expect(auditStatusType(2)).toBe('info')
    expect(auditStatusType(4)).toBe('warning')
  })
})

describe('jobStatusText / jobStatusType', () => {
  it('覆盖全部作业状态', () => {
    expect(jobStatusText('pending')).toBe('待执行')
    expect(jobStatusText('running')).toBe('运行中')
    expect(jobStatusText('paused')).toBe('已暂停')
    expect(jobStatusText('succeeded')).toBe('已成功')
    expect(jobStatusText('partial_failed')).toBe('部分失败')
    expect(jobStatusText('failed')).toBe('已失败')
    expect(jobStatusText('canceled')).toBe('已取消')
    expect(jobStatusText('interrupted')).toBe('已中断')
  })

  it('未知状态回退', () => {
    expect(jobStatusText('weird')).toBe('未知状态')
    expect(jobStatusType('weird')).toBe('default')
  })

  it('颜色映射', () => {
    expect(jobStatusType('running')).toBe('info')
    expect(jobStatusType('succeeded')).toBe('success')
    expect(jobStatusType('partial_failed')).toBe('warning')
    expect(jobStatusType('failed')).toBe('error')
    expect(jobStatusType('pending')).toBe('default')
  })
})

describe('jobTypeText', () => {
  it('覆盖全部作业类型', () => {
    expect(jobTypeText('commit')).toBe('上传代码')
    expect(jobTypeText('submit_audit')).toBe('提交审核')
    expect(jobTypeText('release')).toBe('发布')
    expect(jobTypeText('pipeline')).toBe('全流程')
    expect(jobTypeText('sync_info')).toBe('同步小程序信息')
    expect(jobTypeText('sync_audit_status')).toBe('同步审核状态')
    expect(jobTypeText('undo_audit')).toBe('撤回审核')
    expect(jobTypeText('speedup_audit')).toBe('加急审核')
    expect(jobTypeText('set_domain')).toBe('配置域名')
    expect(jobTypeText('revert')).toBe('版本回退')
    expect(jobTypeText('toggle_visit')).toBe('服务状态')
    expect(jobTypeText('unknown')).toBe('未知类型')
  })
})

describe('itemStatusText / itemStatusType', () => {
  it('覆盖全部条目状态', () => {
    expect(itemStatusText('pending')).toBe('待执行')
    expect(itemStatusText('waiting')).toBe('等待前置')
    expect(itemStatusText('running')).toBe('执行中')
    expect(itemStatusText('succeeded')).toBe('成功')
    expect(itemStatusText('failed')).toBe('失败')
    expect(itemStatusText('skipped')).toBe('已跳过')
    expect(itemStatusText('canceled')).toBe('已取消')
    expect(itemStatusText('other')).toBe('未知状态')
  })

  it('颜色映射', () => {
    expect(itemStatusType('waiting')).toBe('warning')
    expect(itemStatusType('running')).toBe('info')
    expect(itemStatusType('succeeded')).toBe('success')
    expect(itemStatusType('failed')).toBe('error')
  })
})

describe('authorizationStatusText / booleanStatusType', () => {
  it('授权状态文案', () => {
    expect(authorizationStatusText('authorized')).toBe('已授权')
    expect(authorizationStatusText('unauthorized')).toBe('已取消授权')
    expect(authorizationStatusText('x')).toBe('未知状态')
  })

  it('布尔状态颜色', () => {
    expect(booleanStatusType(true)).toBe('success')
    expect(booleanStatusType(false)).toBe('error')
  })
})

describe('formatQuota / usagePercent', () => {
  it('rest 缺失即未探测', () => {
    expect(formatQuota(null, 10)).toBe('未探测')
    expect(formatQuota(undefined)).toBe('未探测')
  })

  it('有 rest 时按需拼接 limit', () => {
    expect(formatQuota(3, 10)).toBe('3 / 10')
    expect(formatQuota(3)).toBe('3')
    expect(formatQuota(3, null)).toBe('3')
  })

  it('百分比计算与夹取', () => {
    expect(usagePercent(1, 4)).toBe(25)
    expect(usagePercent(5, 4)).toBe(100)
    expect(usagePercent(null, 4)).toBe(0)
    expect(usagePercent(1, 0)).toBe(0)
    expect(usagePercent(1, null)).toBe(0)
  })
})

describe('truncateMiddle', () => {
  it('短字符串原样返回', () => {
    expect(truncateMiddle('wx1234567890')).toBe('wx1234567890')
  })

  it('超长字符串保留首尾', () => {
    expect(truncateMiddle('abcdefghijklmnopqrstuvwxyz', 8, 6)).toBe('abcdefgh…uvwxyz')
  })

  it('默认首尾长度', () => {
    expect(truncateMiddle('0123456789abcdefghij')).toBe('01234567…efghij')
  })

  it('边界：长度不超过 head+tail+1 时不截断，超过则截断为 head+tail+1 个字符', () => {
    // 8 + 6 + 1 = 15
    expect(truncateMiddle('abcdefghijklmno', 8, 6)).toBe('abcdefghijklmno')
    expect(truncateMiddle('abcdefghijklmnop', 8, 6)).toBe('abcdefgh…klmnop')
    expect(truncateMiddle('abcdefghijklmnop', 8, 6)).toHaveLength(15)
  })
})
