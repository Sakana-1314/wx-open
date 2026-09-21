<script setup lang="ts">
// 作业详情：概览（状态/进度/errorNote）+ 控制按钮（开始/暂停/恢复/取消/重试失败项）+ 子项明细（可展开看 request/response）。
// 自动刷新：作业处于 running / pending 且开关打开时，每 3 秒刷新作业与「当前页」子项（setTimeout 递归，卸载时清理）。
import { computed, h, onMounted, onUnmounted, ref, watch, type VNodeChild } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NEllipsis,
  NInput,
  NPagination,
  NProgress,
  NSelect,
  NSwitch,
  useDialog,
  useMessage,
  type DataTableColumns,
  type SelectOption,
} from 'naive-ui'

import { client, errorErrcode, errorMessage } from '@/api/client'
import type { components } from '@/api/schema'
import PageHeader from '@/components/PageHeader.vue'
import StatusTag from '@/components/StatusTag.vue'
import {
  EMPTY_TEXT,
  formatDateTime,
  formatRelativeTime,
  itemStatusText,
  jobStatusText,
  jobTypeText,
  truncateMiddle,
  usagePercent,
} from '@/utils/format'
import { useIsMobile } from '@/utils/media'

type Job = components['schemas']['Job']
type JobItem = components['schemas']['JobItem']
type JobItemStatus = components['schemas']['JobItemStatus']

/** 轮询间隔（毫秒）。 */
const POLL_INTERVAL = 3000

/** 子项状态筛选项。 */
const ITEM_STATUSES: JobItemStatus[] = ['pending', 'waiting', 'running', 'succeeded', 'failed', 'skipped', 'canceled']
const ITEM_STATUS_OPTIONS: SelectOption[] = ITEM_STATUSES.map((value) => ({
  label: itemStatusText(value),
  value,
}))

const route = useRoute()
const router = useRouter()
const message = useMessage()
const dialog = useDialog()
const isMobile = useIsMobile()

/** 作业 ID（来自路由参数）。 */
const jobId = computed(() => String(route.params.id ?? ''))

const job = ref<Job | null>(null)
const jobLoading = ref(false)
const items = ref<JobItem[]>([])
const itemTotal = ref(0)
const itemPage = ref(1)
const itemPageSize = ref(10)
const itemStatusFilter = ref<JobItemStatus | null>(null)
/** appid 输入框内容（点查询后生效）。 */
const appidInput = ref('')
const appidFilter = ref('')
const itemsLoading = ref(false)
/** 自动刷新开关（默认开）。 */
const autoRefresh = ref(true)
/** 正在执行的控制操作名。 */
const acting = ref<string | null>(null)

/** 作业是否在途（决定是否轮询）。 */
const jobActive = computed(() => job.value?.status === 'running' || job.value?.status === 'pending')

/** 进度百分比。 */
const progressPercent = computed(() => usagePercent(job.value?.succeeded, job.value?.total))

/** 是否可取消（终态不可取消）。 */
const cancellable = computed(() => {
  const status = job.value?.status
  return status === 'pending' || status === 'running' || status === 'paused'
})

/** 步骤名 → 中文（single 为单步作业的步骤名）。 */
function stepText(step: string): string {
  switch (step) {
    case 'commit':
      return '上传代码'
    case 'privacy_check':
      return '隐私检测'
    case 'submit_audit':
      return '提审'
    case 'release':
      return '发布'
    case 'single':
      return '单步'
    default:
      return step
  }
}

/** 耗时：startedAt → finishedAt（未结束按当前时间算）。 */
function formatDuration(startedAt?: string | null, finishedAt?: string | null): string {
  if (!startedAt) {
    return EMPTY_TEXT
  }
  const start = new Date(startedAt).getTime()
  if (Number.isNaN(start)) {
    return EMPTY_TEXT
  }
  const end = finishedAt ? new Date(finishedAt).getTime() : Date.now()
  if (Number.isNaN(end)) {
    return EMPTY_TEXT
  }
  const seconds = Math.max(0, Math.round((end - start) / 1000))
  if (seconds < 60) {
    return `${seconds} 秒`
  }
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) {
    return `${minutes} 分 ${seconds % 60} 秒`
  }
  const hours = Math.floor(minutes / 60)
  return `${hours} 小时 ${minutes % 60} 分`
}

/** JSON 单元格文本（可能为 null）。 */
function jsonText(value: { [key: string]: unknown } | null | undefined): string {
  if (value === null || value === undefined) {
    return '无（该步骤未产生请求/响应记录）'
  }
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return '内容无法序列化'
  }
}

/** 展开行：request / response 两个 JSON 便于排障。 */
function renderItemExpand(row: JobItem): VNodeChild {
  return h('div', { class: 'expand-panel' }, [
    h('div', { class: 'expand-block' }, [
      h('div', { class: 'expand-title' }, '请求 request'),
      h('pre', { class: 'json-block' }, jsonText(row.request)),
    ]),
    h('div', { class: 'expand-block' }, [
      h('div', { class: 'expand-title' }, '响应 response'),
      h('pre', { class: 'json-block' }, jsonText(row.response)),
    ]),
  ])
}

/** errcode + errmsg（errmsg 已含中文说明与处置建议），超长用 n-ellipsis 折叠。 */
function renderError(row: JobItem): VNodeChild {
  const hasCode = row.errcode !== null && row.errcode !== undefined
  if (row.errmsg === null || row.errmsg === undefined || row.errmsg === '') {
    return hasCode ? String(row.errcode) : EMPTY_TEXT
  }
  const text = hasCode ? `${row.errcode}｜${row.errmsg}` : row.errmsg
  return h(NEllipsis, { style: 'max-width: 300px', lineClamp: 2 }, { default: () => text })
}

/** 下次尝试时间：waiting 项展示「将于 X 重新尝试」。 */
function renderNextRun(row: JobItem): string {
  const value = row.nextRunAt
  if (!value) {
    return EMPTY_TEXT
  }
  if (row.status === 'waiting') {
    return `将于 ${formatRelativeTime(value)}重新尝试（${formatDateTime(value)}）`
  }
  return formatDateTime(value)
}

/** 子项明细列。 */
const itemColumns: DataTableColumns<JobItem> = [
  { type: 'expand', renderExpand: renderItemExpand },
  { title: '步骤', key: 'step', width: 110, render: (row) => stepText(row.step) },
  {
    title: '小程序',
    key: 'appid',
    width: 220,
    render: (row) =>
      h('div', { class: 'cell-stack' }, [
        h('div', { class: 'cell-main' }, truncateMiddle(row.appid, 10, 6)),
        h('div', { class: 'cell-sub' }, row.nickName ?? EMPTY_TEXT),
      ]),
  },
  {
    title: '状态',
    key: 'status',
    width: 110,
    render: (row) => h(StatusTag, { kind: 'item', status: row.status }),
  },
  { title: '尝试次数', key: 'attempt', width: 100 },
  { title: '版本号', key: 'userVersion', width: 150, render: (row) => row.userVersion ?? EMPTY_TEXT },
  {
    title: '审核单号',
    key: 'wxAuditId',
    width: 140,
    render: (row) => (row.wxAuditId === null || row.wxAuditId === undefined ? EMPTY_TEXT : String(row.wxAuditId)),
  },
  { title: '错误码与说明', key: 'errmsg', width: 320, render: (row) => renderError(row) },
  { title: '下次尝试', key: 'nextRunAt', width: 240, render: (row) => renderNextRun(row) },
  { title: '耗时', key: 'duration', width: 130, render: (row) => formatDuration(row.startedAt, row.finishedAt) },
]

/** 错误文案：微信侧失败带上 errcode。 */
function describeError(err: unknown): string {
  const code = errorErrcode(err)
  const text = errorMessage(err)
  return code === null ? text : `${text}（微信 errcode ${code}）`
}

/** 复制文本到剪贴板。 */
async function copyText(text: string, label: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(text)
    message.success(`${label}已复制`)
  } catch {
    message.error('浏览器拒绝访问剪贴板，请手动选择复制')
  }
}

/** 加载作业详情。silent=true 时不显示 loading（轮询用）。 */
async function loadJob(silent = false): Promise<void> {
  if (!silent) {
    jobLoading.value = true
  }
  try {
    const res = await client.GET('/jobs/{id}', { params: { path: { id: jobId.value } } })
    if (res.error || !res.data) {
      message.error(errorMessage(res.error))
      return
    }
    job.value = res.data
  } finally {
    if (!silent) {
      jobLoading.value = false
    }
  }
}

/** 加载当前页子项。 */
async function loadItems(silent = false): Promise<void> {
  if (!silent) {
    itemsLoading.value = true
  }
  try {
    const res = await client.GET('/jobs/{id}/items', {
      params: {
        path: { id: jobId.value },
        query: {
          page: itemPage.value,
          pageSize: itemPageSize.value,
          status: itemStatusFilter.value ?? undefined,
          appid: appidFilter.value.trim() === '' ? undefined : appidFilter.value.trim(),
        },
      },
    })
    if (res.error || !res.data) {
      message.error(errorMessage(res.error))
      return
    }
    items.value = res.data.items
    itemTotal.value = res.data.total
  } finally {
    if (!silent) {
      itemsLoading.value = false
    }
  }
}

// ---- 轮询 ----
let timer: number | null = null

/** 清理定时器。 */
function stopTimer(): void {
  if (timer !== null) {
    window.clearTimeout(timer)
    timer = null
  }
}

/** 按当前状态安排下一次轮询。 */
function scheduleTimer(): void {
  stopTimer()
  if (!autoRefresh.value || !jobActive.value) {
    return
  }
  timer = window.setTimeout(() => {
    timer = null
    void tick()
  }, POLL_INTERVAL)
}

/** 轮询节拍：静默刷新作业与当前页子项。 */
async function tick(): Promise<void> {
  await Promise.all([loadJob(true), loadItems(true)])
  scheduleTimer()
}

/** 刷新作业与子项（用户操作后调用）。 */
async function refreshAll(): Promise<void> {
  await Promise.all([loadJob(true), loadItems(false)])
  scheduleTimer()
}

/** 应用子项筛选（状态下拉自动应用，appid 需点击查询）。 */
async function applyItemFilters(): Promise<void> {
  appidFilter.value = appidInput.value
  itemPage.value = 1
  await loadItems(false)
  scheduleTimer()
}

watch(itemStatusFilter, () => {
  void applyItemFilters()
})

watch([itemPage, itemPageSize], () => {
  void loadItems(false).then(scheduleTimer)
})

watch(autoRefresh, (enabled) => {
  if (enabled) {
    scheduleTimer()
  } else {
    stopTimer()
  }
})

/** 开始作业（pending 时）。 */
async function startJob(): Promise<void> {
  acting.value = 'start'
  try {
    const res = await client.POST('/jobs/{id}/start', { params: { path: { id: jobId.value } } })
    if (res.error || !res.data) {
      message.error(describeError(res.error))
      return
    }
    job.value = res.data
    message.success('作业已开始')
    await refreshAll()
  } finally {
    acting.value = null
  }
}

/** 暂停作业（running 时）。 */
async function pauseJob(): Promise<void> {
  acting.value = 'pause'
  try {
    const res = await client.POST('/jobs/{id}/pause', { params: { path: { id: jobId.value } } })
    if (res.error || !res.data) {
      message.error(describeError(res.error))
      return
    }
    job.value = res.data
    message.success('作业已暂停')
    await refreshAll()
  } finally {
    acting.value = null
  }
}

/** 恢复作业（paused 时）。 */
async function resumeJob(): Promise<void> {
  acting.value = 'resume'
  try {
    const res = await client.POST('/jobs/{id}/resume', { params: { path: { id: jobId.value } } })
    if (res.error || !res.data) {
      message.error(describeError(res.error))
      return
    }
    job.value = res.data
    message.success('作业已恢复运行')
    await refreshAll()
  } finally {
    acting.value = null
  }
}

/** 取消作业（二次确认）。 */
function confirmCancel(): void {
  const current = job.value
  if (!current) {
    return
  }
  dialog.warning({
    title: '取消作业',
    content: `确定取消作业 ${current.id}？未开始的小程序会被跳过，已完成的调用不会回滚。`,
    positiveText: '取消作业',
    negativeText: '返回',
    onPositiveClick: async () => {
      await cancelJob()
    },
  })
}

/** 调用取消接口。 */
async function cancelJob(): Promise<void> {
  acting.value = 'cancel'
  try {
    const res = await client.POST('/jobs/{id}/cancel', { params: { path: { id: jobId.value } } })
    if (res.error || !res.data) {
      message.error(describeError(res.error))
      return
    }
    job.value = res.data
    message.success('作业已取消')
    await refreshAll()
  } finally {
    acting.value = null
  }
}

/** 重试失败项：后端把失败子项重置为 pending。 */
async function retryFailed(): Promise<void> {
  const retryCount = job.value?.failed ?? 0
  acting.value = 'retry'
  try {
    const res = await client.POST('/jobs/{id}/retry-failed', { params: { path: { id: jobId.value } } })
    if (res.error || !res.data) {
      message.error(describeError(res.error))
      return
    }
    job.value = res.data
    message.success(`已重新排队 ${retryCount} 个失败项`)
    await refreshAll()
  } finally {
    acting.value = null
  }
}

onMounted(async () => {
  await Promise.all([loadJob(), loadItems()])
  scheduleTimer()
})

onUnmounted(stopTimer)
</script>

<template>
  <div class="page">
    <page-header title="任务详情" :description="`作业 ${jobId} 的执行概览与子项明细（可展开查看请求/响应）`">
      <template #extra>
        <n-button secondary @click="router.push({ name: 'jobs' })">返回列表</n-button>
        <n-button secondary :loading="jobLoading || itemsLoading" @click="refreshAll">刷新</n-button>
      </template>
    </page-header>

    <n-card :bordered="true" class="section-card" title="作业概览">
      <n-alert v-if="job?.errorNote" type="warning" show-icon class="section-gap">{{ job.errorNote }}</n-alert>

      <div class="info-grid">
        <div class="info-item">
          <div class="info-label">作业 ID</div>
          <div class="info-value">
            <span class="mono">{{ job?.id ?? EMPTY_TEXT }}</span>
            <n-button size="small" secondary @click="copyText(job?.id ?? '', '作业 ID')">复制</n-button>
          </div>
        </div>
        <div class="info-item">
          <div class="info-label">类型</div>
          <div class="info-value">{{ job ? jobTypeText(job.type) : EMPTY_TEXT }}</div>
        </div>
        <div class="info-item">
          <div class="info-label">状态</div>
          <div class="info-value">
            <status-tag v-if="job" kind="job" :status="job.status" />
            <span v-else>{{ EMPTY_TEXT }}</span>
            <span>{{ job ? jobStatusText(job.status) : '' }}</span>
          </div>
        </div>
        <div class="info-item">
          <div class="info-label">执行方式</div>
          <div class="info-value">{{ job ? (job.dryRun ? '试运行（未调用微信）' : '真实执行') : EMPTY_TEXT }}</div>
        </div>
        <div class="info-item">
          <div class="info-label">创建时间</div>
          <div class="info-value">{{ formatDateTime(job?.createdAt) }}</div>
        </div>
        <div class="info-item">
          <div class="info-label">开始 / 结束</div>
          <div class="info-value">
            {{ formatDateTime(job?.startedAt) }} → {{ formatDateTime(job?.finishedAt) }}
          </div>
        </div>
        <div class="info-item">
          <div class="info-label">并发数</div>
          <div class="info-value">{{ job?.concurrency ?? EMPTY_TEXT }}</div>
        </div>
        <div class="info-item">
          <div class="info-label">备注</div>
          <div class="info-value">{{ job?.note ?? EMPTY_TEXT }}</div>
        </div>
      </div>

      <div class="progress-block">
        <div class="progress-text">
          {{ job?.succeeded ?? 0 }} / {{ job?.total ?? 0 }} 已成功 · 失败 {{ job?.failed ?? 0 }} · 跳过
          {{ job?.skipped ?? 0 }} · 等待 {{ job?.waiting ?? 0 }} · 执行中 {{ job?.running ?? 0 }} · 待执行
          {{ job?.pending ?? 0 }}
        </div>
        <n-progress type="line" :percentage="progressPercent" :height="8" :show-indicator="false" />
      </div>

      <div class="actions">
        <n-button v-if="job?.status === 'pending'" type="primary" :loading="acting === 'start'" @click="startJob">
          开始
        </n-button>
        <n-button v-if="job?.status === 'running'" type="primary" :loading="acting === 'pause'" @click="pauseJob">
          暂停
        </n-button>
        <n-button v-if="job?.status === 'paused'" type="primary" :loading="acting === 'resume'" @click="resumeJob">
          恢复
        </n-button>
        <n-button v-if="cancellable" type="error" secondary @click="confirmCancel">取消</n-button>
        <n-button
          v-if="(job?.failed ?? 0) > 0"
          secondary
          :loading="acting === 'retry'"
          @click="retryFailed"
        >
          重试失败项
        </n-button>
        <span class="spacer" />
        <span class="actions-text">自动刷新</span>
        <n-switch v-model:value="autoRefresh" />
      </div>
    </n-card>

    <n-card :bordered="true" class="section-card" title="子项明细">
      <div class="toolbar">
        <n-select
          v-model:value="itemStatusFilter"
          class="filter-select"
          :options="ITEM_STATUS_OPTIONS"
          placeholder="全部状态"
          clearable
        />
        <n-input
          v-model:value="appidInput"
          class="filter-input"
          placeholder="按 appid 精确查询"
          clearable
          @keyup.enter="applyItemFilters"
          @clear="applyItemFilters"
        />
        <n-button secondary :loading="itemsLoading" @click="applyItemFilters">查询</n-button>
        <span class="spacer" />
        <span class="actions-text">共 {{ itemTotal }} 条子项</span>
      </div>

      <n-data-table
        :columns="itemColumns"
        :data="items"
        :loading="itemsLoading"
        :bordered="false"
        :scroll-x="1680"
        :row-key="(row: JobItem) => row.id"
        size="small"
      />

      <div class="pager">
        <n-pagination
          v-model:page="itemPage"
          v-model:page-size="itemPageSize"
          :item-count="itemTotal"
          :page-sizes="[10, 20, 50]"
          :simple="isMobile"
          show-size-picker
        />
      </div>
    </n-card>
  </div>
</template>

<style scoped>
.section-card {
  margin-bottom: 12px;
}

.section-gap {
  margin-bottom: 12px;
}

.info-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 8px 16px;
}

.info-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 6px 0;
  border-bottom: 1px solid #eef2f8;
}

.info-label {
  width: 110px;
  flex-shrink: 0;
  font-size: 14px;
  font-weight: 600;
  color: #3d4a5c;
}

.info-value {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 14px;
  color: #1f2937;
  word-break: break-all;
}

.mono {
  word-break: break-all;
}

.progress-block {
  margin-top: 12px;
}

.progress-text {
  font-size: 14px;
  color: #1f2937;
  margin-bottom: 6px;
}

.actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  margin-top: 12px;
}

.actions-text {
  font-size: 14px;
  color: #1f2937;
}

.spacer {
  flex: 1;
}

.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  margin-bottom: 12px;
}

.filter-select {
  width: 180px;
}

.filter-input {
  width: 240px;
}

.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}

.cell-stack {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.cell-main {
  font-size: 14px;
  color: #1f2937;
  word-break: break-all;
}

.cell-sub {
  font-size: 14px;
  color: #3d4a5c;
}

.expand-panel {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 12px;
  padding: 8px 4px;
}

.expand-block {
  min-width: 0;
}

.expand-title {
  font-size: 14px;
  font-weight: 600;
  color: #17233d;
  margin-bottom: 6px;
}

.json-block {
  margin: 0;
  max-height: 260px;
  overflow: auto;
  font-size: 13px;
  line-height: 1.5;
  color: #1f2937;
  background: #f7f9fc;
  border-radius: 6px;
  padding: 8px;
}

@media (max-width: 820px) {
  .info-label {
    width: 100%;
  }

  .info-item {
    flex-direction: column;
    gap: 4px;
  }

  .filter-select,
  .filter-input {
    width: 100%;
  }

  .toolbar :deep(.n-button) {
    width: 100%;
  }

  .spacer {
    display: none;
  }

  .actions :deep(.n-button) {
    width: 100%;
  }

  .pager {
    justify-content: center;
  }
}
</style>
