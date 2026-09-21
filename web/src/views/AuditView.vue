<script setup lang="ts">
// 审核管理：审核状态对账、服务商级提审/加急额度、撤回与加急审核、单小程序审核历史。
// 额度是「服务商级、旗下小程序共用」，因此任意一个已授权小程序查到的都是同一份数据。
import { computed, h, onMounted, onUnmounted, ref, watch } from 'vue'
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NDrawer,
  NDrawerContent,
  NEllipsis,
  NInput,
  NInputNumber,
  NModal,
  NSelect,
  NSwitch,
  NTag,
  NTooltip,
  useDialog,
  useMessage,
  type DataTableColumns,
  type PaginationProps,
} from 'naive-ui'

import { client, errorErrcode, errorMessage } from '@/api/client'
import PageHeader from '@/components/PageHeader.vue'
import StatusTag from '@/components/StatusTag.vue'
import type { components } from '@/api/schema'
import { useIsMobile } from '@/utils/media'
import {
  EMPTY_TEXT,
  auditStatusText,
  formatDateTime,
  formatQuota,
  truncateMiddle,
} from '@/utils/format'

type AuditRecord = components['schemas']['AuditRecord']
type AuditStatus = components['schemas']['AuditStatus']
type AuditQuota = components['schemas']['AuditQuota']
type SyncSummary = components['schemas']['SyncSummary']

/** 审核来源（api 接口调用 / event 回调事件 / poll 轮询对账）文案与标签色。 */
const SOURCE_TEXT: Record<string, string> = {
  api: '接口调用',
  event: '回调事件',
  poll: '轮询对账',
}
const SOURCE_TYPE: Record<string, 'info' | 'success' | 'default'> = {
  api: 'info',
  event: 'success',
  poll: 'default',
}

const message = useMessage()
const dialog = useDialog()
const isMobile = useIsMobile()

const loading = ref(false)
const items = ref<AuditRecord[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const appidFilter = ref('')
const statusFilter = ref<AuditStatus | null>(null)

/** appid → 昵称，用于在列表里显示可读名称（审核列表不回昵称，需从授权方清单匹配）。 */
const authorizerNames = ref<Map<string, string>>(new Map())

const syncing = ref(false)
const syncSummary = ref<SyncSummary | null>(null)
const syncFailures = computed(() => (syncSummary.value?.details ?? []).filter((detail) => !detail.ok))

const quota = ref<AuditQuota | null>(null)
const quotaLoading = ref(false)
/** 额度卡片状态：idle 未查询 / ok 已查询 / no-authorizer 尚无已授权小程序。 */
const quotaState = ref<'idle' | 'ok' | 'no-authorizer'>('idle')
/** 本次额度探测所用的代表小程序。 */
const quotaAppid = ref('')

const autoRefresh = ref(false)
let timer: number | null = null
/** appid 输入框的防抖句柄，避免每敲一个字符就发一次请求。 */
let filterTimer: number | null = null

const historyOpen = ref(false)
const historyLoading = ref(false)
const historyAppid = ref('')
const historyItems = ref<AuditRecord[]>([])

const speedupVisible = ref(false)
const speedupSubmitting = ref(false)
const speedupAppid = ref('')
const speedupAuditId = ref<number | null>(null)
/** 加急弹窗内的提示（未探测到额度 / 89405 额度用尽）。 */
const speedupNotice = ref('')
/** 撤回审核的本地已用次数提示：优先读取后端 409 的 message，不自行统计。 */
const undoHint = ref('本地暂未记录撤回次数，若超出官方限制微信会返回 87013。')

const statusOptions = ([0, 1, 2, 3, 4] as const).map((value) => ({
  label: auditStatusText(value),
  value,
}))

/** 额度卡片数据。 */
const quotaCells = computed(() => [
  {
    key: 'rest',
    label: '提审额度剩余',
    value: formatQuota(quota.value?.rest, quota.value?.limit),
  },
  {
    key: 'speedup',
    label: '加急额度剩余',
    value: formatQuota(quota.value?.speedupRest, quota.value?.speedupLimit),
  },
])

/** 提审额度用尽（或尚未探测到）时给出处置建议。 */
const quotaWarning = computed<string | null>(() => {
  if (quotaState.value !== 'ok') {
    return null
  }
  const rest = quota.value?.rest
  if (rest === null || rest === undefined || rest <= 0) {
    return '提审额度已用尽或尚未探测：额度用尽时提审会返回 85085，需在「小程序服务商助手」申请临时额度后再试。'
  }
  return null
})

/** 行内小程序展示名（匹配到昵称时优先显示昵称）。 */
function appidLabel(appid: string): string {
  return authorizerNames.value.get(appid) ?? truncateMiddle(appid)
}

/** 审核历史列。 */
const historyColumns: DataTableColumns<AuditRecord> = [
  {
    title: '审核状态',
    key: 'status',
    width: 130,
    render: (row) => h(StatusTag, { kind: 'audit', status: row.status }),
  },
  { title: '版本', key: 'userVersion', width: 150, render: (row) => row.userVersion ?? EMPTY_TEXT },
  {
    title: '原因',
    key: 'reason',
    minWidth: 200,
    render: (row) =>
      row.reason
        ? h(NEllipsis, { style: 'max-width: 260px' }, { default: () => row.reason })
        : EMPTY_TEXT,
  },
  {
    title: '提交时间',
    key: 'submitTime',
    width: 180,
    render: (row) => formatDateTime(row.submitTime),
  },
  {
    title: '状态时间',
    key: 'statusTime',
    width: 180,
    render: (row) => formatDateTime(row.statusTime),
  },
]

const columns: DataTableColumns<AuditRecord> = [
  {
    title: '小程序',
    key: 'appid',
    width: 230,
    render: (row) => {
      const nickName = authorizerNames.value.get(row.appid)
      return h('div', { class: 'cell-stack' }, [
        h('span', { class: 'cell-main' }, nickName ?? truncateMiddle(row.appid)),
        nickName ? h('span', { class: 'cell-sub' }, row.appid) : null,
      ])
    },
  },
  {
    title: '审核 ID',
    key: 'auditId',
    width: 130,
    render: (row) => String(row.auditId),
  },
  { title: '版本', key: 'userVersion', width: 140, render: (row) => row.userVersion ?? EMPTY_TEXT },
  {
    title: '审核状态',
    key: 'status',
    width: 120,
    render: (row) => h(StatusTag, { kind: 'audit', status: row.status }),
  },
  {
    title: '原因',
    key: 'reason',
    minWidth: 200,
    render: (row) => {
      // 只有被拒绝 / 延后才有原因，其余状态不展示。
      if (row.status !== 1 && row.status !== 4) {
        return EMPTY_TEXT
      }
      return row.reason
        ? h(NEllipsis, { style: 'max-width: 240px' }, { default: () => row.reason })
        : EMPTY_TEXT
    },
  },
  {
    title: '截图',
    key: 'screenshotMediaIds',
    width: 120,
    render: (row) => {
      const ids = row.screenshotMediaIds ?? []
      if (ids.length === 0) {
        return EMPTY_TEXT
      }
      return h(
        NTooltip,
        { trigger: 'hover' },
        {
          trigger: () =>
            h(
              NTag,
              { size: 'small', type: 'info', bordered: false },
              { default: () => `${ids.length} 张` },
            ),
          default: () => '可通过获取永久素材接口拉取截图（media_id 已存库）',
        },
      )
    },
  },
  {
    title: '提交时间',
    key: 'submitTime',
    width: 180,
    render: (row) => formatDateTime(row.submitTime),
  },
  {
    title: '状态时间',
    key: 'statusTime',
    width: 180,
    render: (row) => formatDateTime(row.statusTime),
  },
  {
    title: '来源',
    key: 'source',
    width: 110,
    render: (row) => {
      const source = row.source
      if (!source) {
        return EMPTY_TEXT
      }
      return h(
        NTag,
        { size: 'small', type: SOURCE_TYPE[source] ?? 'default', bordered: false },
        { default: () => SOURCE_TEXT[source] ?? source },
      )
    },
  },
  {
    title: '操作',
    key: 'actions',
    width: 280,
    fixed: 'right',
    render: (row) =>
      h('div', { class: 'row-actions' }, [
        h(
          NTooltip,
          { trigger: 'hover' },
          {
            trigger: () =>
              h(
                NButton,
                {
                  size: 'small',
                  type: 'error',
                  secondary: true,
                  onClick: () => confirmUndo(row),
                },
                { default: () => '撤回审核' },
              ),
            default: () => '官方限制：每账号每天最多 5 次、每月最多 10 次，超限返回 87013',
          },
        ),
        h(
          NButton,
          {
            size: 'small',
            secondary: true,
            disabled: row.status !== 2,
            onClick: () => void openSpeedup(row),
          },
          { default: () => '加急审核' },
        ),
        h(
          NButton,
          { size: 'small', onClick: () => void openHistory(row) },
          { default: () => '历史' },
        ),
      ]),
  },
]

/** 分页配置（窄屏用 simple 精简样式）。 */
const pagination = computed<PaginationProps>(() => ({
  page: page.value,
  pageSize: pageSize.value,
  itemCount: total.value,
  showSizePicker: true,
  pageSizes: [10, 20, 50],
  simple: isMobile.value,
  prefix: ({ itemCount }) => `共 ${itemCount} 条`,
  onChange: (value: number) => {
    page.value = value
    void loadList()
  },
  onUpdatePageSize: (value: number) => {
    pageSize.value = value
    page.value = 1
    void loadList()
  },
}))

/** 加载授权方清单（用于昵称匹配）。 */
async function loadAuthorizerNames(): Promise<void> {
  const res = await client.GET('/authorizers', { params: { query: { pageSize: 200 } } })
  if (res.error || !res.data) {
    return
  }
  const map = new Map<string, string>()
  res.data.items.forEach((item) => {
    if (item.nickName) {
      map.set(item.appid, item.nickName)
    }
  })
  authorizerNames.value = map
}

/** 加载审核列表。 */
async function loadList(): Promise<void> {
  if (loading.value) {
    return
  }
  loading.value = true
  try {
    const res = await client.GET('/audits', {
      params: {
        query: {
          page: page.value,
          pageSize: pageSize.value,
          appid: appidFilter.value.trim() === '' ? undefined : appidFilter.value.trim(),
          status: statusFilter.value ?? undefined,
        },
      },
    })
    if (res.error || !res.data) {
      message.error(errorMessage(res.error))
      return
    }
    items.value = res.data.items
    total.value = res.data.total
  } finally {
    loading.value = false
  }
}

/**
 * 查询服务商额度。appid 为空时先用 GET /authorizers?pageSize=1 取任意一个已授权小程序作代表
 * （额度是服务商级、旗下小程序共用，用哪个小程序查结果一致）。
 */
async function queryQuota(appid?: string, silent = false): Promise<AuditQuota | null> {
  let target = appid?.trim() ?? ''
  if (target === '') {
    const listed = await client.GET('/authorizers', { params: { query: { pageSize: 1 } } })
    if (listed.error || !listed.data) {
      if (!silent) {
        message.error(errorMessage(listed.error))
      }
      return null
    }
    const first = listed.data.items[0]
    if (!first) {
      quotaState.value = 'no-authorizer'
      quotaAppid.value = ''
      if (!silent) {
        message.warning('尚无已授权小程序，无法查询服务商额度')
      }
      return null
    }
    target = first.appid
  }

  quotaLoading.value = true
  try {
    const res = await client.GET('/audits/{appid}/quota', { params: { path: { appid: target } } })
    if (res.error || !res.data) {
      if (!silent) {
        message.error(errorMessage(res.error))
      }
      return null
    }
    quota.value = res.data
    quotaAppid.value = target
    quotaState.value = 'ok'
    return res.data
  } finally {
    quotaLoading.value = false
  }
}

/** 同步审核状态：body 传空对象表示对全部已授权小程序对账。 */
async function syncAudits(): Promise<void> {
  syncing.value = true
  try {
    const res = await client.POST('/audits/sync', { body: {} })
    if (res.error || !res.data) {
      message.error(errorMessage(res.error))
      return
    }
    syncSummary.value = res.data
    if (res.data.failed > 0) {
      message.warning(
        `同步完成：共 ${res.data.total} 个，成功 ${res.data.succeeded}，失败 ${res.data.failed}`,
      )
    } else {
      message.success(`同步完成：共 ${res.data.total} 个，全部成功`)
    }
    await loadList()
    await queryQuota(undefined, true)
  } finally {
    syncing.value = false
  }
}

/** 撤回审核：二次确认后调接口；87013 时把后端 message 记为本地已用次数提示。 */
function confirmUndo(row: AuditRecord): void {
  dialog.warning({
    title: '撤回审核',
    content: () =>
      h('div', { class: 'dialog-lines' }, [
        h('p', null, `确定撤回「${appidLabel(row.appid)}」当前提交的审核申请吗？`),
        h('p', null, '官方限制：每账号每天最多 5 次、每月最多 10 次，超限返回 87013。'),
        h('p', null, undoHint.value),
      ]),
    positiveText: '撤回',
    negativeText: '取消',
    onPositiveClick: async () => {
      const res = await client.POST('/audits/{appid}/undo', {
        params: { path: { appid: row.appid } },
      })
      if (res.error || !res.data) {
        const errcode = errorErrcode(res.error)
        const text = errorMessage(res.error)
        undoHint.value =
          errcode === 87013
            ? `微信返回 87013：本账号撤回次数已超出官方限制（每天 5 次、每月 10 次）。后端提示：${text}`
            : `上次撤回未成功：${text}`
        message.error(text)
        return
      }
      if (res.data.ok) {
        message.success(res.data.message ?? '已撤回审核')
      } else {
        const text = res.data.message ?? res.data.errmsg ?? '微信未返回原因'
        undoHint.value = `上次撤回未成功：${text}`
        message.warning(text)
      }
      await loadList()
    },
  })
}

/** 加急审核：先确认加急额度 > 0，再弹窗填写 auditId。 */
async function openSpeedup(row: AuditRecord): Promise<void> {
  speedupNotice.value = ''
  const fresh = await queryQuota(row.appid)
  const rest = fresh?.speedupRest
  if (rest !== null && rest !== undefined && rest <= 0) {
    message.error('加急额度已用尽（89405），请等待额度恢复后再试')
    return
  }
  if (fresh === null) {
    speedupNotice.value = '未能探测到加急额度（查询额度失败），仍可提交；若额度用尽微信会返回 89405。'
  } else if (rest === null || rest === undefined) {
    speedupNotice.value = '加急额度尚未探测到，仍可提交；若额度用尽微信会返回 89405。'
  }
  speedupAppid.value = row.appid
  speedupAuditId.value = row.auditId
  speedupVisible.value = true
}

/** 提交加急。 */
async function submitSpeedup(): Promise<void> {
  const auditId = speedupAuditId.value
  if (auditId === null || !Number.isFinite(auditId)) {
    message.warning('请填写有效的 auditId')
    return
  }
  speedupSubmitting.value = true
  speedupNotice.value = ''
  try {
    const res = await client.POST('/audits/{appid}/speed-up', {
      params: { path: { appid: speedupAppid.value } },
      body: { auditId },
    })
    if (res.error || !res.data) {
      const errcode = errorErrcode(res.error)
      const text = errorMessage(res.error)
      speedupNotice.value =
        errcode === 89405
          ? `加急额度已用尽（89405），请等待额度恢复后再试。后端提示：${text}`
          : text
      return
    }
    speedupVisible.value = false
    message.success('已加急，预计 2-12 小时内审完')
    await loadList()
    await queryQuota(speedupAppid.value, true)
  } finally {
    speedupSubmitting.value = false
  }
}

/** 打开审核历史抽屉。 */
async function openHistory(row: AuditRecord): Promise<void> {
  historyAppid.value = row.appid
  historyItems.value = []
  historyOpen.value = true
  historyLoading.value = true
  try {
    const res = await client.GET('/audits/{appid}/history', {
      params: { path: { appid: row.appid } },
    })
    if (res.error || !res.data) {
      message.error(errorMessage(res.error))
      return
    }
    historyItems.value = res.data
  } finally {
    historyLoading.value = false
  }
}

function stopTimer(): void {
  if (timer !== null) {
    window.clearInterval(timer)
    timer = null
  }
}

/** 自动刷新：有待审核记录时用于对账，3 秒轮询一次。 */
function startTimer(): void {
  stopTimer()
  timer = window.setInterval(() => {
    void loadList()
  }, 3000)
}

watch(autoRefresh, (enabled) => {
  if (enabled) {
    startTimer()
  } else {
    stopTimer()
  }
})

watch([appidFilter, statusFilter], () => {
  page.value = 1
  if (filterTimer !== null) {
    window.clearTimeout(filterTimer)
  }
  filterTimer = window.setTimeout(() => {
    void loadList()
  }, 300)
})

onMounted(async () => {
  await loadAuthorizerNames()
  await loadList()
})

onUnmounted(() => {
  stopTimer()
  if (filterTimer !== null) {
    window.clearTimeout(filterTimer)
  }
})
</script>

<template>
  <div class="page">
    <page-header
      title="审核管理"
      description="审核状态对账、服务商额度查询、撤回与加急审核、单小程序审核历史"
    >
      <template #extra>
        <n-button type="primary" :loading="syncing" @click="syncAudits">同步审核状态</n-button>
      </template>
    </page-header>

    <div class="page-toolbar">
      <n-input
        v-model:value="appidFilter"
        class="toolbar-input"
        placeholder="按 appid 过滤"
        clearable
      />
      <n-select
        v-model:value="statusFilter"
        class="toolbar-select"
        :options="statusOptions"
        placeholder="全部审核状态"
        clearable
      />
      <div class="spacer" />
      <span class="toolbar-label">自动刷新（3 秒）</span>
      <n-switch v-model:value="autoRefresh" />
    </div>

    <n-alert
      v-if="syncSummary"
      class="summary-alert"
      :type="syncSummary.failed > 0 ? 'warning' : 'success'"
      show-icon
    >
      同步结果：共 {{ syncSummary.total }} 个小程序，成功 {{ syncSummary.succeeded }}，失败
      {{ syncSummary.failed }}。
      <template v-if="syncFailures.length > 0">
        <span v-for="(item, index) in syncFailures" :key="index">
          {{ item.key }}：{{ item.errmsg ?? '未知原因' }}<br />
        </span>
      </template>
    </n-alert>

    <n-card class="section-card" title="服务商额度" :bordered="true">
      <template #header-extra>
        <n-button size="small" :loading="quotaLoading" @click="queryQuota()">查询额度</n-button>
      </template>

      <div class="quota-grid">
        <div v-for="cell in quotaCells" :key="cell.key" class="quota-cell">
          <div class="quota-label">{{ cell.label }}</div>
          <div class="quota-value">{{ cell.value }}</div>
        </div>
      </div>

      <p class="note-text">
        额度为服务商级、旗下小程序共用，任意一个已授权小程序查到的都是同一份数据。
        <template v-if="quotaAppid">本次代表小程序：{{ appidLabel(quotaAppid) }}。</template>
        <template v-if="quota?.queriedAt">探测时间：{{ formatDateTime(quota.queriedAt) }}。</template>
        <template v-if="quotaState === 'idle'">点「查询额度」才会计入一次微信侧探测。</template>
      </p>

      <n-alert v-if="quotaState === 'no-authorizer'" type="warning" show-icon>
        尚无已授权小程序，无法查询服务商额度：请先在「小程序管理」页完成授权并同步。
      </n-alert>
      <n-alert v-else-if="quotaWarning" class="quota-alert" type="warning" show-icon>
        {{ quotaWarning }}
      </n-alert>
    </n-card>

    <n-card title="审核记录" :bordered="true">
      <n-data-table
        :columns="columns"
        :data="items"
        :loading="loading"
        :bordered="false"
        :row-key="(row: AuditRecord) => String(row.auditId)"
        :pagination="pagination"
        :remote="true"
        :scroll-x="1560"
        size="small"
      />
    </n-card>

    <n-drawer v-model:show="historyOpen" :width="isMobile ? '94vw' : 720">
      <n-drawer-content :title="`审核历史 · ${appidLabel(historyAppid)}`" closable>
        <p class="note-text">该小程序的全部审核记录，按时间倒序。</p>
        <n-data-table
          :columns="historyColumns"
          :data="historyItems"
          :loading="historyLoading"
          :bordered="false"
          :row-key="(row: AuditRecord) => String(row.auditId)"
          :scroll-x="760"
          size="small"
        />
      </n-drawer-content>
    </n-drawer>

    <n-modal v-model:show="speedupVisible" preset="card" title="加急审核" class="form-modal">
      <div class="form-block">
        <div class="form-label">小程序</div>
        <div class="form-text">{{ appidLabel(speedupAppid) }}（{{ speedupAppid }}）</div>
      </div>
      <div class="form-block">
        <div class="form-label">审核 ID</div>
        <n-input-number
          v-model:value="speedupAuditId"
          class="full-width"
          :min="1"
          :precision="0"
          placeholder="填写要加急的 auditId"
        />
        <p class="note-text">默认填入该行的 auditId，可改为其它正在审核中的 auditId。</p>
      </div>
      <p class="note-text">
        加急后预计 2-12 小时内审完；加急额度用尽时微信返回 89405。
        <template v-if="quota">
          当前加急额度剩余：{{ formatQuota(quota.speedupRest, quota.speedupLimit) }}。
        </template>
      </p>
      <n-alert v-if="speedupNotice" class="speedup-alert" type="warning" show-icon>
        {{ speedupNotice }}
      </n-alert>
      <template #footer>
        <div class="modal-footer">
          <n-button @click="speedupVisible = false">取消</n-button>
          <n-button type="primary" :loading="speedupSubmitting" @click="submitSpeedup">
            提交加急
          </n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.section-card {
  margin-bottom: 12px;
}

.summary-alert {
  margin-bottom: 12px;
}

.toolbar-input {
  width: 240px;
}

.toolbar-select {
  width: 180px;
}

.toolbar-label {
  font-size: 14px;
  color: #1f2937;
}

.quota-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.quota-cell {
  flex: 1 1 200px;
  min-width: 160px;
  padding: 10px 12px;
  border: 1px solid #dbe5f1;
  border-radius: 8px;
}

.quota-label {
  font-size: 14px;
  font-weight: 600;
  color: #3d4a5c;
}

.quota-value {
  margin-top: 6px;
  font-size: 22px;
  font-weight: 650;
  color: #17233d;
}

.quota-alert,
.speedup-alert {
  margin-top: 10px;
}

/* 说明一律用正文色 14px，不使用灰色小字。 */
.note-text {
  margin: 10px 0 0;
  font-size: 14px;
  line-height: 1.6;
  color: #3d4a5c;
}

.cell-stack {
  display: flex;
  flex-direction: column;
}

.cell-main {
  color: #17233d;
  font-weight: 600;
}

.cell-sub {
  font-size: 14px;
  color: #3d4a5c;
  word-break: break-all;
}

.row-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.form-block {
  margin-bottom: 12px;
}

.form-label {
  margin-bottom: 6px;
  font-size: 14px;
  font-weight: 600;
  color: #3d4a5c;
}

.form-text {
  font-size: 14px;
  color: #1f2937;
  word-break: break-all;
}

.full-width {
  width: 100%;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

:deep(.dialog-lines p) {
  margin: 0 0 6px;
  font-size: 14px;
  line-height: 1.6;
  color: #3d4a5c;
}

/* 弹窗宽度带 max-width: 94vw，窄屏不溢出 */
.form-modal {
  width: 520px;
  max-width: 94vw;
}

@media (max-width: 820px) {
  .toolbar-input,
  .toolbar-select {
    width: 100%;
  }

  .modal-footer :deep(.n-button) {
    flex: 1;
  }
}
</style>
