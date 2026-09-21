<script setup lang="ts">
// 批量任务列表：筛选 + 进度 + 行内控制（暂停/恢复/取消/重试失败项）+ 运行中作业自动轮询。
// 轮询策略：仅当列表中存在 running / pending 的作业且「自动刷新」开关打开时，每 3 秒重新拉取当前页；
// 使用 setTimeout 递归（不用 setInterval），页面卸载时清理，避免请求重叠与内存泄漏。
import { computed, h, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import {
  NButton,
  NCard,
  NDataTable,
  NIcon,
  NPagination,
  NProgress,
  NSelect,
  NSwitch,
  NTag,
  useDialog,
  useMessage,
  type DataTableColumns,
  type SelectOption,
} from 'naive-ui'
import { AddOutline, CopyOutline } from '@vicons/ionicons5'

import { client, errorErrcode, errorMessage } from '@/api/client'
import type { components } from '@/api/schema'
import PageHeader from '@/components/PageHeader.vue'
import StatusTag from '@/components/StatusTag.vue'
import { EMPTY_TEXT, formatDateTime, jobStatusText, jobTypeText, truncateMiddle, usagePercent } from '@/utils/format'
import { useIsMobile } from '@/utils/media'

type Job = components['schemas']['Job']
type JobType = components['schemas']['JobType']
type JobStatus = components['schemas']['JobStatus']

/** 轮询间隔（毫秒）。 */
const POLL_INTERVAL = 3000

/** 支持批量下发的作业类型（顺序即下拉顺序）。 */
const JOB_TYPES: JobType[] = [
  'commit',
  'submit_audit',
  'release',
  'pipeline',
  'sync_info',
  'sync_audit_status',
  'undo_audit',
  'speedup_audit',
  'set_domain',
  'revert',
  'toggle_visit',
]

/** 作业状态枚举（顺序即下拉顺序）。 */
const JOB_STATUSES: JobStatus[] = [
  'pending',
  'running',
  'paused',
  'succeeded',
  'partial_failed',
  'failed',
  'canceled',
  'interrupted',
]

const router = useRouter()
const message = useMessage()
const dialog = useDialog()
const isMobile = useIsMobile()

const typeOptions: SelectOption[] = JOB_TYPES.map((value) => ({ label: jobTypeText(value), value }))
const statusOptions: SelectOption[] = JOB_STATUSES.map((value) => ({ label: jobStatusText(value), value }))

const jobs = ref<Job[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)
const typeFilter = ref<JobType | null>(null)
const statusFilter = ref<JobStatus | null>(null)
const loading = ref(false)
/** 当前页是否存在 running / pending 的作业，决定是否继续轮询。 */
const hasActive = ref(false)
/** 自动刷新开关（默认开）。 */
const autoRefresh = ref(true)
/** 正在执行行内操作的作业 id。 */
const actingJobId = ref<string | null>(null)

/** 非空状态下拉值（null → undefined，避免把 null 当成查询参数发出去）。 */
const queryType = computed<JobType | undefined>(() => typeFilter.value ?? undefined)
const queryStatus = computed<JobStatus | undefined>(() => statusFilter.value ?? undefined)

/** 作业表格列。 */
const columns: DataTableColumns<Job> = [
  {
    title: '作业 ID',
    key: 'id',
    width: 190,
    render: (row) =>
      h('div', { class: 'id-cell' }, [
        h('span', { class: 'id-text' }, truncateMiddle(row.id, 8, 6)),
        h(
          NButton,
          { size: 'small', secondary: true, onClick: () => void copyText(row.id, '作业 ID') },
          {
            icon: () => h(NIcon, null, { default: () => h(CopyOutline) }),
            default: () => '复制',
          },
        ),
      ]),
  },
  { title: '类型', key: 'type', width: 120, render: (row) => jobTypeText(row.type) },
  {
    title: '状态',
    key: 'status',
    width: 110,
    render: (row) => h(StatusTag, { kind: 'job', status: row.status }),
  },
  {
    title: '进度',
    key: 'progress',
    width: 170,
    render: (row) =>
      h('div', { class: 'progress-cell' }, [
        h('span', { class: 'progress-text' }, `${row.succeeded} / ${row.total}`),
        h(NProgress, {
          type: 'line',
          percentage: usagePercent(row.succeeded, row.total),
          height: 6,
          showIndicator: false,
        }),
      ]),
  },
  { title: '成功', key: 'succeeded', width: 80 },
  { title: '失败', key: 'failed', width: 80 },
  { title: '跳过', key: 'skipped', width: 80 },
  {
    title: '试运行',
    key: 'dryRun',
    width: 100,
    render: (row) =>
      row.dryRun
        ? h(NTag, { type: 'warning', size: 'small', round: true, bordered: false }, { default: () => '试运行' })
        : EMPTY_TEXT,
  },
  { title: '创建时间', key: 'createdAt', width: 170, render: (row) => formatDateTime(row.createdAt) },
  {
    title: '操作',
    key: 'actions',
    width: 330,
    fixed: 'right',
    render: (row) =>
      h('div', { class: 'row-actions' }, [
        h(
          NButton,
          { size: 'small', secondary: true, onClick: () => void goDetail(row) },
          { default: () => '详情' },
        ),
        row.status === 'running'
          ? h(
              NButton,
              {
                size: 'small',
                secondary: true,
                loading: actingJobId.value === row.id,
                onClick: () => void pauseJob(row),
              },
              { default: () => '暂停' },
            )
          : null,
        row.status === 'paused'
          ? h(
              NButton,
              {
                size: 'small',
                secondary: true,
                loading: actingJobId.value === row.id,
                onClick: () => void resumeJob(row),
              },
              { default: () => '恢复' },
            )
          : null,
        isCancellable(row)
          ? h(
              NButton,
              {
                size: 'small',
                type: 'error',
                secondary: true,
                onClick: () => confirmCancel(row),
              },
              { default: () => '取消' },
            )
          : null,
        row.failed > 0 && row.status !== 'running'
          ? h(
              NButton,
              {
                size: 'small',
                secondary: true,
                loading: actingJobId.value === row.id,
                onClick: () => void retryFailed(row),
              },
              { default: () => '重试失败项' },
            )
          : null,
      ]),
  },
]

/** 是否允许取消（终态作业不可取消）。 */
function isCancellable(row: Job): boolean {
  return row.status === 'pending' || row.status === 'running' || row.status === 'paused'
}

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

/** 跳转作业详情。 */
async function goDetail(row: Job): Promise<void> {
  await router.push({ name: 'job-detail', params: { id: row.id } })
}

// ---- 轮询：只有存在在途作业时才每 3 秒拉一次，避免无谓请求 ----
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
  if (!autoRefresh.value || !hasActive.value) {
    return
  }
  timer = window.setTimeout(() => {
    timer = null
    void tick()
  }, POLL_INTERVAL)
}

/** 轮询节拍：静默刷新列表后继续安排下一拍。 */
async function tick(): Promise<void> {
  await loadJobs(false)
  scheduleTimer()
}

/** 加载作业列表。showLoading=false 用于轮询时的静默刷新。 */
async function loadJobs(showLoading = true): Promise<void> {
  if (showLoading) {
    loading.value = true
  }
  try {
    const res = await client.GET('/jobs', {
      params: {
        query: {
          page: page.value,
          pageSize: pageSize.value,
          type: queryType.value,
          status: queryStatus.value,
        },
      },
    })
    if (res.error || !res.data) {
      hasActive.value = false
      message.error(errorMessage(res.error))
      return
    }
    jobs.value = res.data.items
    total.value = res.data.total
    hasActive.value = res.data.items.some((job) => job.status === 'running' || job.status === 'pending')
  } finally {
    if (showLoading) {
      loading.value = false
    }
  }
}

/** 筛选/分页变化后重新加载并重置轮询节奏。 */
async function reload(): Promise<void> {
  await loadJobs(true)
  scheduleTimer()
}

watch([typeFilter, statusFilter], () => {
  page.value = 1
  void reload()
})

watch([page, pageSize], () => {
  void reload()
})

watch(autoRefresh, (enabled) => {
  if (enabled) {
    scheduleTimer()
  } else {
    stopTimer()
  }
})

/** 暂停作业。 */
async function pauseJob(row: Job): Promise<void> {
  actingJobId.value = row.id
  try {
    const res = await client.POST('/jobs/{id}/pause', { params: { path: { id: row.id } } })
    if (res.error || !res.data) {
      message.error(describeError(res.error))
      return
    }
    message.success('作业已暂停')
    await reload()
  } finally {
    actingJobId.value = null
  }
}

/** 恢复作业。 */
async function resumeJob(row: Job): Promise<void> {
  actingJobId.value = row.id
  try {
    const res = await client.POST('/jobs/{id}/resume', { params: { path: { id: row.id } } })
    if (res.error || !res.data) {
      message.error(describeError(res.error))
      return
    }
    message.success('作业已恢复运行')
    await reload()
  } finally {
    actingJobId.value = null
  }
}

/** 取消作业（二次确认）。 */
function confirmCancel(row: Job): void {
  dialog.warning({
    title: '取消作业',
    content: `确定取消作业 ${row.id}？未开始的小程序将被跳过，已完成的调用不会回滚。`,
    positiveText: '取消作业',
    negativeText: '返回',
    onPositiveClick: async () => {
      await cancelJob(row)
    },
  })
}

/** 调用取消接口。 */
async function cancelJob(row: Job): Promise<void> {
  actingJobId.value = row.id
  try {
    const res = await client.POST('/jobs/{id}/cancel', { params: { path: { id: row.id } } })
    if (res.error || !res.data) {
      message.error(describeError(res.error))
      return
    }
    message.success('作业已取消')
    await reload()
  } finally {
    actingJobId.value = null
  }
}

/** 重试失败项：后端把失败子项重置为 pending，返回重置后的作业。 */
async function retryFailed(row: Job): Promise<void> {
  const retryCount = row.failed
  actingJobId.value = row.id
  try {
    const res = await client.POST('/jobs/{id}/retry-failed', { params: { path: { id: row.id } } })
    if (res.error || !res.data) {
      message.error(describeError(res.error))
      return
    }
    message.success(`已重新排队 ${retryCount} 个失败项`)
    await reload()
  } finally {
    actingJobId.value = null
  }
}

onMounted(async () => {
  await loadJobs(true)
  scheduleTimer()
})

onUnmounted(stopTimer)
</script>

<template>
  <div class="page">
    <page-header title="批量任务" description="作业列表、进度控制与失败重试（在途作业每 3 秒自动刷新）">
      <template #extra>
        <n-button type="primary" @click="router.push({ name: 'job-create' })">
          <template #icon>
            <n-icon><add-outline /></n-icon>
          </template>
          新建批量任务
        </n-button>
      </template>
    </page-header>

    <n-card :bordered="true">
      <div class="toolbar">
        <n-select
          v-model:value="typeFilter"
          class="filter-select"
          :options="typeOptions"
          placeholder="全部类型"
          clearable
        />
        <n-select
          v-model:value="statusFilter"
          class="filter-select"
          :options="statusOptions"
          placeholder="全部状态"
          clearable
        />
        <n-button secondary :loading="loading" @click="reload">刷新</n-button>
        <span class="spacer" />
        <span class="toolbar-text">自动刷新</span>
        <n-switch v-model:value="autoRefresh" />
        <span class="toolbar-text">共 {{ total }} 个作业</span>
      </div>

      <n-data-table
        :columns="columns"
        :data="jobs"
        :loading="loading"
        :bordered="false"
        :scroll-x="1600"
        :row-key="(row: Job) => row.id"
        size="small"
      />

      <div class="pager">
        <n-pagination
          v-model:page="page"
          v-model:page-size="pageSize"
          :item-count="total"
          :page-sizes="[10, 20, 50]"
          :simple="isMobile"
          show-size-picker
        />
      </div>
    </n-card>
  </div>
</template>

<style scoped>
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

.spacer {
  flex: 1;
}

.toolbar-text {
  font-size: 14px;
  color: #1f2937;
}

.id-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}

.id-text {
  font-size: 14px;
  color: #1f2937;
  word-break: break-all;
}

.progress-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.progress-text {
  font-size: 14px;
  color: #1f2937;
}

.row-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}

@media (max-width: 820px) {
  .filter-select {
    width: 100%;
  }

  .toolbar :deep(.n-button) {
    width: 100%;
  }

  .spacer {
    display: none;
  }

  .pager {
    justify-content: center;
  }
}
</style>
