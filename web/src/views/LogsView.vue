<script setup lang="ts">
// 日志页：三个页签 —— 微信调用日志 / 回调事件 / 操作日志。
// 微信调用日志保留了每个请求的原始返回码与官方中文含义，是排查 85085 / 61039 / 9402202 的第一现场。
import { computed, h, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import {
  NButton,
  NCard,
  NDataTable,
  NEllipsis,
  NInput,
  NInputNumber,
  NSelect,
  NSwitch,
  NTabPane,
  NTabs,
  NTag,
  useMessage,
  type DataTableColumns,
  type PaginationProps,
} from 'naive-ui'

import { client, errorMessage } from '@/api/client'
import PageHeader from '@/components/PageHeader.vue'
import StatusTag from '@/components/StatusTag.vue'
import type { components } from '@/api/schema'
import { useIsMobile } from '@/utils/media'
import { EMPTY_TEXT, formatDateTime, truncateMiddle } from '@/utils/format'

type ApiCallLog = components['schemas']['ApiCallLog']
type CallbackEvent = components['schemas']['CallbackEvent']
type OperationLog = components['schemas']['OperationLog']
type CallbackKind = components['schemas']['CallbackKind']

/** 错误分类 → 标签色（蓝可重试 / 橙频次令牌 / 红环境或永久失败 / 灰未归类）。 */
const ERROR_CLASS_TYPE: Record<string, 'info' | 'warning' | 'error' | 'default'> = {
  retryable: 'info',
  token_expired: 'warning',
  rate_limited: 'warning',
  environment: 'error',
  permanent: 'error',
  wechat_unknown: 'default',
}
/** 错误分类 → 中文文案。 */
const ERROR_CLASS_TEXT: Record<string, string> = {
  retryable: '可重试',
  token_expired: '令牌过期',
  rate_limited: '频次或额度',
  environment: '环境问题',
  permanent: '永久失败',
  wechat_unknown: '未归类',
}

type TabKey = 'calls' | 'callbacks' | 'operations'

const message = useMessage()
const isMobile = useIsMobile()

const activeTab = ref<TabKey>('calls')
/** 每页均可选 3 秒自动刷新，默认关闭。 */
const autoRefresh = reactive<Record<TabKey, boolean>>({
  calls: false,
  callbacks: false,
  operations: false,
})
let timer: number | null = null
/** 筛选输入的防抖句柄，避免每敲一个字符就发一次请求。 */
const filterTimers = new Map<TabKey, number>()

/** 对某个页签的筛选变更做 300ms 防抖后再查询。 */
function scheduleLoad(tab: TabKey, reload: () => void): void {
  const existed = filterTimers.get(tab)
  if (existed !== undefined) {
    window.clearTimeout(existed)
  }
  filterTimers.set(
    tab,
    window.setTimeout(() => {
      filterTimers.delete(tab)
      reload()
    }, 300),
  )
}

/** 调用日志状态。 */
const callsLoading = ref(false)
const calls = ref<ApiCallLog[]>([])
const callsTotal = ref(0)
const callsPage = ref(1)
const callsPageSize = ref(20)
const callAppid = ref('')
const callJobId = ref('')
const callEndpoint = ref('')
const callErrcode = ref<number | null>(null)

/** 回调事件状态。 */
const callbacksLoading = ref(false)
const callbacks = ref<CallbackEvent[]>([])
const callbacksTotal = ref(0)
const callbacksPage = ref(1)
const callbacksPageSize = ref(20)
const callbackKind = ref<CallbackKind | null>(null)
const callbackAppid = ref('')
const callbackInfoType = ref('')

/** 操作日志状态。 */
const operationsLoading = ref(false)
const operations = ref<OperationLog[]>([])
const operationsTotal = ref(0)
const operationsPage = ref(1)
const operationsPageSize = ref(20)
const operationAction = ref('')

const kindOptions = [
  { label: '组件授权事件（component）', value: 'component' as CallbackKind },
  { label: '消息与事件（message）', value: 'message' as CallbackKind },
]

/** 分页配置（窄屏用 simple 精简样式）。 */
function makePagination(
  page: typeof callsPage,
  pageSize: typeof callsPageSize,
  total: typeof callsTotal,
  reload: () => void,
) {
  return computed<PaginationProps>(() => ({
    page: page.value,
    pageSize: pageSize.value,
    itemCount: total.value,
    showSizePicker: true,
    pageSizes: [10, 20, 50],
    simple: isMobile.value,
    prefix: ({ itemCount }) => `共 ${itemCount} 条`,
    onChange: (value: number) => {
      page.value = value
      reload()
    },
    onUpdatePageSize: (value: number) => {
      pageSize.value = value
      page.value = 1
      reload()
    },
  }))
}

/** JSON 友好展示（保留缩进；空值给出占位）。 */
function prettyJson(value: unknown): string {
  if (value === null || value === undefined) {
    return '（无）'
  }
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

/** 展开区里的一段 JSON 预格式化文本。 */
function jsonBlock(title: string, value: unknown) {
  return h('div', { class: 'expand-block' }, [
    h('div', { class: 'expand-title' }, title),
    h('pre', { class: 'json-block' }, prettyJson(value)),
  ])
}

const callsColumns: DataTableColumns<ApiCallLog> = [
  { type: 'expand', renderExpand: (row) => h('div', null, [jsonBlock('请求', row.request), jsonBlock('响应', row.response)]) },
  {
    title: '时间',
    key: 'createdAt',
    width: 180,
    render: (row) => formatDateTime(row.createdAt),
  },
  {
    title: '令牌',
    key: 'scope',
    width: 120,
    render: (row) =>
      h(
        NTag,
        { size: 'small', type: row.scope === 'component' ? 'info' : 'default', bordered: false },
        { default: () => (row.scope === 'component' ? '组件令牌' : '授权方令牌') },
      ),
  },
  {
    title: '接口',
    key: 'endpoint',
    minWidth: 260,
    render: (row) =>
      h('span', { class: 'endpoint-cell' }, `${(row.method ?? 'GET').toUpperCase()} ${row.endpoint}`),
  },
  {
    title: 'HTTP',
    key: 'httpStatus',
    width: 90,
    render: (row) => (row.httpStatus === undefined ? EMPTY_TEXT : String(row.httpStatus)),
  },
  {
    title: '结果',
    key: 'ok',
    width: 100,
    render: (row) =>
      h(StatusTag, { kind: 'boolean', status: row.ok, label: row.ok ? '成功' : '失败' }),
  },
  {
    title: '返回码 / 说明',
    key: 'errcode',
    minWidth: 240,
    render: (row) => {
      const errcode = row.errcode === null || row.errcode === undefined ? '' : `${row.errcode} `
      const text = `${errcode}${row.errmsg ?? ''}`.trim()
      if (text === '') {
        return EMPTY_TEXT
      }
      return h(NEllipsis, { style: 'max-width: 260px' }, { default: () => text })
    },
  },
  {
    title: '错误分类',
    key: 'errorClass',
    width: 130,
    render: (row) => {
      const value = row.errorClass
      if (!value) {
        return EMPTY_TEXT
      }
      return h(
        NTag,
        { size: 'small', type: ERROR_CLASS_TYPE[value] ?? 'default', bordered: false },
        { default: () => ERROR_CLASS_TEXT[value] ?? value },
      )
    },
  },
  {
    title: '耗时',
    key: 'durationMs',
    width: 100,
    render: (row) => (row.durationMs === undefined ? EMPTY_TEXT : `${row.durationMs} ms`),
  },
  {
    title: '作业 ID',
    key: 'jobId',
    width: 150,
    render: (row) => (row.jobId ? truncateMiddle(row.jobId, 8, 4) : EMPTY_TEXT),
  },
]

const callbacksColumns: DataTableColumns<CallbackEvent> = [
  {
    type: 'expand',
    renderExpand: (row) =>
      h('div', { class: 'expand-block' }, [
        h('div', { class: 'expand-title' }, `解密后的明文报文（${row.infoType ?? row.event ?? '未知类型'}）`),
        h('pre', { class: 'json-block' }, row.decrypted ?? '（无）'),
      ]),
  },
  {
    title: '时间',
    key: 'receivedAt',
    width: 180,
    render: (row) => formatDateTime(row.receivedAt),
  },
  {
    title: '来源',
    key: 'kind',
    width: 130,
    render: (row) =>
      h(
        NTag,
        { size: 'small', type: row.kind === 'component' ? 'info' : 'default', bordered: false },
        { default: () => (row.kind === 'component' ? '授权事件 URL' : '消息事件 URL') },
      ),
  },
  {
    title: '小程序',
    key: 'appid',
    width: 200,
    render: (row) => (row.appid ? truncateMiddle(row.appid, 10, 6) : EMPTY_TEXT),
  },
  {
    title: 'infoType / event',
    key: 'infoType',
    minWidth: 200,
    render: (row) => row.infoType ?? row.event ?? EMPTY_TEXT,
  },
  {
    title: '签名',
    key: 'signatureOk',
    width: 120,
    render: (row) =>
      h(StatusTag, {
        kind: 'boolean',
        status: row.signatureOk,
        label: row.signatureOk ? '签名通过' : '签名失败',
      }),
  },
  {
    title: '处理',
    key: 'processed',
    width: 110,
    render: (row) =>
      h(StatusTag, {
        kind: 'boolean',
        status: row.processed,
        label: row.processed ? '已处理' : '未处理',
      }),
  },
  {
    title: '处理说明',
    key: 'processNote',
    minWidth: 240,
    render: (row) =>
      row.processNote
        ? h(NEllipsis, { style: 'max-width: 260px' }, { default: () => row.processNote })
        : EMPTY_TEXT,
  },
]

const operationsColumns: DataTableColumns<OperationLog> = [
  {
    type: 'expand',
    renderExpand: (row) => jsonBlock('明细', row.detail),
  },
  {
    title: '时间',
    key: 'createdAt',
    width: 180,
    render: (row) => formatDateTime(row.createdAt),
  },
  { title: '操作人', key: 'actor', width: 140 },
  { title: '操作', key: 'action', width: 180 },
  {
    title: '对象类型',
    key: 'targetType',
    width: 140,
    render: (row) => row.targetType ?? EMPTY_TEXT,
  },
  {
    title: '对象',
    key: 'targetId',
    width: 200,
    render: (row) => (row.targetId ? truncateMiddle(row.targetId, 12, 6) : EMPTY_TEXT),
  },
  { title: 'IP', key: 'ip', width: 160, render: (row) => row.ip ?? EMPTY_TEXT },
]

const callsPagination = makePagination(callsPage, callsPageSize, callsTotal, () => void loadCalls())
const callbacksPagination = makePagination(
  callbacksPage,
  callbacksPageSize,
  callbacksTotal,
  () => void loadCallbacks(),
)
const operationsPagination = makePagination(
  operationsPage,
  operationsPageSize,
  operationsTotal,
  () => void loadOperations(),
)

/** 微信调用日志。 */
async function loadCalls(): Promise<void> {
  if (callsLoading.value) {
    return
  }
  callsLoading.value = true
  try {
    const res = await client.GET('/logs/api-calls', {
      params: {
        query: {
          page: callsPage.value,
          pageSize: callsPageSize.value,
          appid: callAppid.value.trim() === '' ? undefined : callAppid.value.trim(),
          jobId: callJobId.value.trim() === '' ? undefined : callJobId.value.trim(),
          endpoint: callEndpoint.value.trim() === '' ? undefined : callEndpoint.value.trim(),
          errcode: callErrcode.value ?? undefined,
        },
      },
    })
    if (res.error || !res.data) {
      message.error(errorMessage(res.error))
      return
    }
    calls.value = res.data.items
    callsTotal.value = res.data.total
  } finally {
    callsLoading.value = false
  }
}

/** 回调事件。 */
async function loadCallbacks(): Promise<void> {
  if (callbacksLoading.value) {
    return
  }
  callbacksLoading.value = true
  try {
    const res = await client.GET('/logs/callbacks', {
      params: {
        query: {
          page: callbacksPage.value,
          pageSize: callbacksPageSize.value,
          kind: callbackKind.value ?? undefined,
          appid: callbackAppid.value.trim() === '' ? undefined : callbackAppid.value.trim(),
          infoType: callbackInfoType.value.trim() === ''
            ? undefined
            : callbackInfoType.value.trim(),
        },
      },
    })
    if (res.error || !res.data) {
      message.error(errorMessage(res.error))
      return
    }
    callbacks.value = res.data.items
    callbacksTotal.value = res.data.total
  } finally {
    callbacksLoading.value = false
  }
}

/** 操作日志。 */
async function loadOperations(): Promise<void> {
  if (operationsLoading.value) {
    return
  }
  operationsLoading.value = true
  try {
    const res = await client.GET('/logs/operations', {
      params: {
        query: {
          page: operationsPage.value,
          pageSize: operationsPageSize.value,
          action: operationAction.value.trim() === '' ? undefined : operationAction.value.trim(),
        },
      },
    })
    if (res.error || !res.data) {
      message.error(errorMessage(res.error))
      return
    }
    operations.value = res.data.items
    operationsTotal.value = res.data.total
  } finally {
    operationsLoading.value = false
  }
}

/** 刷新当前页签（手动刷新按钮与自动刷新共用）。 */
function refreshActive(): void {
  if (activeTab.value === 'calls') {
    void loadCalls()
  } else if (activeTab.value === 'callbacks') {
    void loadCallbacks()
  } else {
    void loadOperations()
  }
}

function stopTimer(): void {
  if (timer !== null) {
    window.clearInterval(timer)
    timer = null
  }
}

/** 单个 3 秒定时器，仅刷新「当前页签 + 该页签已开启自动刷新」的数据。 */
function startTimer(): void {
  stopTimer()
  timer = window.setInterval(() => {
    if (autoRefresh[activeTab.value]) {
      refreshActive()
    }
  }, 3000)
}

watch([callAppid, callJobId, callEndpoint, callErrcode], () => {
  callsPage.value = 1
  scheduleLoad('calls', () => void loadCalls())
})

watch([callbackKind, callbackAppid, callbackInfoType], () => {
  callbacksPage.value = 1
  scheduleLoad('callbacks', () => void loadCallbacks())
})

watch(operationAction, () => {
  operationsPage.value = 1
  scheduleLoad('operations', () => void loadOperations())
})

watch(activeTab, (tab) => {
  if (tab === 'calls' && calls.value.length === 0) {
    void loadCalls()
  } else if (tab === 'callbacks' && callbacks.value.length === 0) {
    void loadCallbacks()
  } else if (tab === 'operations' && operations.value.length === 0) {
    void loadOperations()
  }
})

onMounted(async () => {
  startTimer()
  await loadCalls()
})

onUnmounted(() => {
  stopTimer()
  filterTimers.forEach((handle) => window.clearTimeout(handle))
  filterTimers.clear()
})
</script>

<template>
  <div class="page">
    <page-header
      title="日志"
      description="微信调用日志、回调事件与后台操作日志，用于排查微信返回码与回调处理问题"
    >
      <template #extra>
        <n-button @click="refreshActive">刷新</n-button>
      </template>
    </page-header>

    <n-card :bordered="true">
      <n-tabs v-model:value="activeTab" type="line" animated>
        <n-tab-pane name="calls" tab="微信调用日志">
          <p class="note-text">
            这里能看到每个微信请求的原始返回码与官方中文含义，排查 85085/61039/9402202
            等问题时先看这里。展开任意一行可查看该次请求与响应的完整报文。
          </p>

          <div class="page-toolbar">
            <n-input
              v-model:value="callAppid"
              class="toolbar-input"
              placeholder="按 appid 过滤"
              clearable
            />
            <n-input
              v-model:value="callJobId"
              class="toolbar-input"
              placeholder="按作业 ID 过滤"
              clearable
            />
            <n-input
              v-model:value="callEndpoint"
              class="toolbar-input"
              placeholder="按接口路径过滤（如 /wxa/commit）"
              clearable
            />
            <n-input-number
              v-model:value="callErrcode"
              class="toolbar-number"
              placeholder="按 errcode 过滤"
              :min="0"
              :precision="0"
              clearable
            />
            <div class="spacer" />
            <span class="toolbar-label">自动刷新（3 秒）</span>
            <n-switch v-model:value="autoRefresh.calls" />
          </div>

          <n-data-table
            :columns="callsColumns"
            :data="calls"
            :loading="callsLoading"
            :bordered="false"
            :row-key="(row: ApiCallLog) => String(row.id)"
            :pagination="callsPagination"
            :remote="true"
            :scroll-x="1700"
            size="small"
          />
        </n-tab-pane>

        <n-tab-pane name="callbacks" tab="回调事件">
          <p class="note-text">
            授权事件 URL 与消息事件 URL 收到的每一次回调都记录在此；签名通过但未处理的记录，
            「处理说明」里会写明原因。展开可查看解密后的明文报文（原样保留换行）。
          </p>

          <div class="page-toolbar">
            <n-select
              v-model:value="callbackKind"
              class="toolbar-input"
              :options="kindOptions"
              placeholder="全部来源"
              clearable
            />
            <n-input
              v-model:value="callbackAppid"
              class="toolbar-input"
              placeholder="按 appid 过滤"
              clearable
            />
            <n-input
              v-model:value="callbackInfoType"
              class="toolbar-input"
              placeholder="按 infoType 过滤（如 authorized）"
              clearable
            />
            <div class="spacer" />
            <span class="toolbar-label">自动刷新（3 秒）</span>
            <n-switch v-model:value="autoRefresh.callbacks" />
          </div>

          <n-data-table
            :columns="callbacksColumns"
            :data="callbacks"
            :loading="callbacksLoading"
            :bordered="false"
            :row-key="(row: CallbackEvent) => String(row.id)"
            :pagination="callbacksPagination"
            :remote="true"
            :scroll-x="1300"
            size="small"
          />
        </n-tab-pane>

        <n-tab-pane name="operations" tab="操作日志">
          <p class="note-text">
            后台管理动作的留档：谁、什么时候、对哪个对象做了什么。展开可查看明细 JSON。
          </p>

          <div class="page-toolbar">
            <n-input
              v-model:value="operationAction"
              class="toolbar-input"
              placeholder="按操作名过滤（如 update_settings）"
              clearable
            />
            <div class="spacer" />
            <span class="toolbar-label">自动刷新（3 秒）</span>
            <n-switch v-model:value="autoRefresh.operations" />
          </div>

          <n-data-table
            :columns="operationsColumns"
            :data="operations"
            :loading="operationsLoading"
            :bordered="false"
            :row-key="(row: OperationLog) => String(row.id)"
            :pagination="operationsPagination"
            :remote="true"
            :scroll-x="1100"
            size="small"
          />
        </n-tab-pane>
      </n-tabs>
    </n-card>
  </div>
</template>

<style scoped>
.toolbar-input,
.toolbar-number {
  width: 220px;
}

.toolbar-label {
  font-size: 14px;
  color: #1f2937;
}

/* 说明一律用正文色 14px，不使用灰色小字。 */
.note-text {
  margin: 0 0 12px;
  font-size: 14px;
  line-height: 1.6;
  color: #3d4a5c;
}

.endpoint-cell {
  word-break: break-all;
}

.expand-block {
  margin-bottom: 8px;
}

.expand-title {
  font-size: 14px;
  font-weight: 600;
  color: #17233d;
  margin-bottom: 4px;
}

/* 保留原始换行（回调明文报文依赖换行） */
.json-block {
  margin: 0;
  padding: 10px;
  max-height: 320px;
  overflow: auto;
  background: #f5f8fd;
  border: 1px solid #dbe5f1;
  border-radius: 6px;
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: 13px;
  line-height: 1.6;
  color: #1f2937;
  white-space: pre-wrap;
  word-break: break-all;
}

.hint-alert {
  margin-top: 12px;
}

@media (max-width: 820px) {
  .toolbar-input,
  .toolbar-number {
    width: 100%;
  }
}
</style>
