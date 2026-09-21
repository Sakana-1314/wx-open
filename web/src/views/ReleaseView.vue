<script setup lang="ts">
// 发布管理：查看体验版/线上版、全量发布、分阶段发布（灰度）、灰度计划与取消、
// 历史版本回退、体验版二维码，以及发布台账。
import { computed, h, onMounted, onUnmounted, ref, watch } from 'vue'
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NEmpty,
  NInput,
  NInputNumber,
  NModal,
  NSelect,
  NSwitch,
  NTag,
  useDialog,
  useMessage,
  type DataTableColumns,
  type PaginationProps,
} from 'naive-ui'

import { client, errorErrcode, errorMessage } from '@/api/client'
import PageHeader from '@/components/PageHeader.vue'
import type { components } from '@/api/schema'
import { useIsMobile } from '@/utils/media'
import { EMPTY_TEXT, formatDateTime, truncateMiddle } from '@/utils/format'

type ReleaseRecord = components['schemas']['ReleaseRecord']
type VersionInfo = components['schemas']['VersionInfo']
type HistoryVersion = components['schemas']['HistoryVersion']
type GrayReleasePlan = components['schemas']['GrayReleasePlan']
type QRCodeImage = components['schemas']['QRCodeImage']

/** 台账动作 → 中文文案与标签色。 */
const ACTION_TEXT: Record<string, string> = {
  release: '全量发布',
  grayrelease: '分阶段发布',
  revert: '版本回退',
}
const ACTION_TYPE: Record<string, 'info' | 'warning' | 'error'> = {
  release: 'info',
  grayrelease: 'warning',
  revert: 'error',
}

/** 灰度计划状态 → 中文文案（0 初始 / 1 执行中 / 2 暂停中 / 3 执行完毕 / 4 被删除）。 */
const GRAY_STATUS_TEXT: Record<number, string> = {
  0: '初始',
  1: '执行中',
  2: '暂停中',
  3: '执行完毕',
  4: '被删除',
}

const message = useMessage()
const dialog = useDialog()
const isMobile = useIsMobile()

const authorizers = ref<{ label: string; value: string }[]>([])
const selectedAppid = ref<string | null>(null)
/** 查询版本用的 appid 输入（从选择器选中时自动带上）。 */
const appidInput = ref('')
const versionLoading = ref(false)
const version = ref<VersionInfo | null>(null)

const actionLoading = ref(false)
const grayPlan = ref<GrayReleasePlan | null>(null)
const grayPlanLoaded = ref(false)
const historyVersions = ref<HistoryVersion[]>([])
const historyLoaded = ref(false)

const grayVisible = ref(false)
const graySubmitting = ref(false)
const grayPercentage = ref<number | null>(10)
const supportDebugerFirst = ref(false)
const supportExperiencerFirst = ref(false)

const qrVisible = ref(false)
const qrLoading = ref(false)
const qrPath = ref('')
const qrCode = ref<QRCodeImage | null>(null)

const ledgerLoading = ref(false)
const ledgerItems = ref<ReleaseRecord[]>([])
const ledgerTotal = ref(0)
const ledgerPage = ref(1)
const ledgerPageSize = ref(20)
const ledgerAppid = ref('')
/** 台账 appid 过滤的防抖句柄。 */
let ledgerTimer: number | null = null

/** 体验版二维码 data URL。 */
const qrSrc = computed<string>(() =>
  qrCode.value ? `data:${qrCode.value.contentType};base64,${qrCode.value.base64}` : '',
)

/** 灰度计划状态文案。 */
const grayStatusText = computed<string>(() => {
  const status = grayPlan.value?.status
  if (status === null || status === undefined) {
    return EMPTY_TEXT
  }
  return GRAY_STATUS_TEXT[status] ?? '未知状态'
})

const authorizerOptions = computed(() => authorizers.value)

const historyColumns: DataTableColumns<HistoryVersion> = [
  { title: '版本号', key: 'appVersion', width: 110, render: (row) => String(row.appVersion) },
  { title: '版本', key: 'userVersion', width: 160, render: (row) => row.userVersion ?? EMPTY_TEXT },
  { title: '描述', key: 'userDesc', minWidth: 200, render: (row) => row.userDesc ?? EMPTY_TEXT },
  {
    title: '提交时间',
    key: 'commitTime',
    width: 180,
    render: (row) => formatDateTime(row.commitTime),
  },
  {
    title: '操作',
    key: 'actions',
    width: 150,
    fixed: 'right',
    render: (row) =>
      h(
        NButton,
        {
          size: 'small',
          type: 'error',
          secondary: true,
          onClick: () => confirmRevertTo(row),
        },
        { default: () => '回退到此版本' },
      ),
  },
]

const ledgerColumns: DataTableColumns<ReleaseRecord> = [
  {
    title: '小程序',
    key: 'appid',
    width: 220,
    render: (row) => truncateMiddle(row.appid, 10, 6),
  },
  {
    title: '动作',
    key: 'action',
    width: 130,
    render: (row) =>
      h(
        NTag,
        { size: 'small', type: ACTION_TYPE[row.action] ?? 'default', bordered: false },
        { default: () => ACTION_TEXT[row.action] ?? row.action },
      ),
  },
  { title: '版本', key: 'userVersion', width: 150, render: (row) => row.userVersion ?? EMPTY_TEXT },
  {
    title: '灰度比例',
    key: 'grayPercentage',
    width: 110,
    render: (row) => (row.grayPercentage === null || row.grayPercentage === undefined
      ? EMPTY_TEXT
      : `${row.grayPercentage}%`),
  },
  {
    title: '发布时间',
    key: 'releaseTime',
    width: 180,
    render: (row) => formatDateTime(row.releaseTime),
  },
  { title: '备注', key: 'note', minWidth: 200, render: (row) => row.note ?? EMPTY_TEXT },
]

const ledgerPagination = computed<PaginationProps>(() => ({
  page: ledgerPage.value,
  pageSize: ledgerPageSize.value,
  itemCount: ledgerTotal.value,
  showSizePicker: true,
  pageSizes: [10, 20, 50],
  simple: isMobile.value,
  prefix: ({ itemCount }) => `共 ${itemCount} 条`,
  onChange: (value: number) => {
    ledgerPage.value = value
    void loadLedger()
  },
  onUpdatePageSize: (value: number) => {
    ledgerPageSize.value = value
    ledgerPage.value = 1
    void loadLedger()
  },
}))

/** 加载小程序选项（发布操作全部按单个 appid 进行）。 */
async function loadAuthorizers(): Promise<void> {
  const res = await client.GET('/authorizers', { params: { query: { pageSize: 200 } } })
  if (res.error || !res.data) {
    message.error(errorMessage(res.error))
    return
  }
  authorizers.value = res.data.items.map((item) => ({
    label: item.nickName ? `${item.nickName}（${truncateMiddle(item.appid, 8, 4)}）` : item.appid,
    value: item.appid,
  }))
}

/** 查询体验版与线上版信息。 */
async function queryVersion(target?: string): Promise<void> {
  const appid = (target ?? appidInput.value).trim()
  if (appid === '') {
    message.warning('请先填写或选择小程序 appid')
    return
  }
  appidInput.value = appid
  versionLoading.value = true
  try {
    const res = await client.GET('/releases/{appid}/version', { params: { path: { appid } } })
    if (res.error || !res.data) {
      message.error(errorMessage(res.error))
      return
    }
    version.value = res.data
  } finally {
    versionLoading.value = false
  }
}

/** 发布线上版本：全量且立即生效，必须二次确认。 */
function confirmRelease(): void {
  const appid = requireAppid()
  if (appid === null) {
    return
  }
  dialog.warning({
    title: '发布线上版本',
    content: () =>
      h('div', { class: 'dialog-lines' }, [
        h('p', null, `确定发布「${selectedLabel.value}」的线上版本吗？`),
        h('p', null, '发布的是最后一个审核通过的版本，全量发布且立即生效，用户会立刻使用新版本。'),
        h('p', null, '若需控制影响范围，请改用「分阶段发布」。'),
      ]),
    positiveText: '发布',
    negativeText: '取消',
    onPositiveClick: () => runAction(appid, 'release'),
  })
}

/** 分阶段发布（灰度）。 */
async function submitGray(): Promise<void> {
  const appid = selectedAppid.value
  if (appid === null) {
    return
  }
  const percentage = grayPercentage.value
  if (percentage === null || !Number.isFinite(percentage) || percentage < 0 || percentage > 100) {
    message.warning('灰度比例必须是 0-100 之间的整数')
    return
  }
  graySubmitting.value = true
  try {
    const res = await client.POST('/releases/{appid}/gray', {
      params: { path: { appid } },
      body: {
        grayPercentage: percentage,
        supportDebugerFirst: supportDebugerFirst.value,
        supportExperiencerFirst: supportExperiencerFirst.value,
      },
    })
    if (res.error || !res.data) {
      message.error(errorMessage(res.error))
      return
    }
    grayVisible.value = false
    message.success(`已提交分阶段发布：灰度 ${percentage}%`)
    await loadGrayPlan()
    await loadLedger()
  } finally {
    graySubmitting.value = false
  }
}

/** 查询灰度计划。 */
async function loadGrayPlan(): Promise<void> {
  const appid = selectedAppid.value
  if (appid === null) {
    return
  }
  actionLoading.value = true
  try {
    const res = await client.GET('/releases/{appid}/gray-plan', { params: { path: { appid } } })
    if (res.error || !res.data) {
      message.error(errorMessage(res.error))
      return
    }
    grayPlan.value = res.data
    grayPlanLoaded.value = true
  } finally {
    actionLoading.value = false
  }
}

/** 取消分阶段发布（危险操作，二次确认）。 */
function confirmRevertGray(): void {
  const appid = requireAppid()
  if (appid === null) {
    return
  }
  dialog.warning({
    title: '取消分阶段发布',
    content: () =>
      h('div', { class: 'dialog-lines' }, [
        h('p', null, `确定取消「${selectedLabel.value}」当前的分阶段发布吗？`),
        h('p', null, '取消后灰度计划会被删除，已灰度到的用户不会自动回退。'),
      ]),
    positiveText: '取消分阶段发布',
    negativeText: '返回',
    onPositiveClick: () => runAction(appid, 'revert-gray'),
  })
}

/** 查询可回退的历史版本（官方最多保留 5 个）。 */
async function loadHistoryVersions(): Promise<void> {
  const appid = selectedAppid.value
  if (appid === null) {
    return
  }
  actionLoading.value = true
  try {
    const res = await client.GET('/releases/{appid}/history-versions', {
      params: { path: { appid } },
    })
    if (res.error || !res.data) {
      message.error(errorMessage(res.error))
      return
    }
    historyVersions.value = res.data
    historyLoaded.value = true
  } finally {
    actionLoading.value = false
  }
}

/** 回退到指定历史版本。 */
function confirmRevertTo(row: HistoryVersion): void {
  const appid = requireAppid()
  if (appid === null) {
    return
  }
  dialog.warning({
    title: '回退到此版本',
    content: () =>
      h('div', { class: 'dialog-lines' }, [
        h('p', null, `确定把「${selectedLabel.value}」回退到版本 ${row.appVersion}（${row.userVersion ?? '未命名版本'}）吗？`),
        h('p', null, '回退同样会立即生效；同一版本回退过一次后不能再重复回退（微信返回 87012）。'),
      ]),
    positiveText: '回退',
    negativeText: '取消',
    onPositiveClick: async () => {
      actionLoading.value = true
      try {
        const res = await client.POST('/releases/{appid}/revert', {
          params: { path: { appid } },
          body: { appVersion: row.appVersion },
        })
        handleResult(res.error, res.data, '已提交回退，请稍后刷新线上版本')
      } finally {
        actionLoading.value = false
      }
    },
  })
}

/** 回退到上一个线上版本。 */
function confirmRevertPrev(): void {
  const appid = requireAppid()
  if (appid === null) {
    return
  }
  dialog.warning({
    title: '回退到上一个版本',
    content: () =>
      h('div', { class: 'dialog-lines' }, [
        h('p', null, `确定把「${selectedLabel.value}」回退到上一个线上版本吗？`),
        h('p', null, '无上一个线上版本时无法回退；同一版本不能重复回退（微信返回 87012）。'),
      ]),
    positiveText: '回退',
    negativeText: '取消',
    onPositiveClick: () => runAction(appid, 'revert-previous'),
  })
}

/** 生成体验版二维码。 */
async function loadQRCode(): Promise<void> {
  const appid = selectedAppid.value
  if (appid === null) {
    return
  }
  qrLoading.value = true
  try {
    const res = await client.GET('/releases/{appid}/trial-qrcode', {
      params: { path: { appid }, query: { path: qrPath.value.trim() || undefined } },
    })
    if (res.error || !res.data) {
      message.error(errorMessage(res.error))
      return
    }
    qrCode.value = res.data
  } finally {
    qrLoading.value = false
  }
}

/** 打开二维码弹窗并立即取一次码。 */
async function openQRCode(): Promise<void> {
  qrCode.value = null
  qrVisible.value = true
  await loadQRCode()
}

/** 加载发布台账。 */
async function loadLedger(): Promise<void> {
  ledgerLoading.value = true
  try {
    const res = await client.GET('/releases', {
      params: {
        query: {
          page: ledgerPage.value,
          pageSize: ledgerPageSize.value,
          appid: ledgerAppid.value.trim() === '' ? undefined : ledgerAppid.value.trim(),
        },
      },
    })
    if (res.error || !res.data) {
      message.error(errorMessage(res.error))
      return
    }
    ledgerItems.value = res.data.items
    ledgerTotal.value = res.data.total
  } finally {
    ledgerLoading.value = false
  }
}

/** 已选小程序的展示名。 */
const selectedLabel = computed<string>(() => {
  const appid = selectedAppid.value
  if (appid === null) {
    return EMPTY_TEXT
  }
  return authorizers.value.find((item) => item.value === appid)?.label ?? appid
})

/** 取当前选中 appid，未选择时提示并返回 null。 */
function requireAppid(): string | null {
  const appid = selectedAppid.value
  if (appid === null) {
    message.warning('请先选择小程序')
    return null
  }
  return appid
}

/** 统一处理发布类动作的成功/失败返回（含 87012 重复回退的明确提示）。 */
function handleResult(
  error: unknown,
  result: components['schemas']['AuditActionResult'] | undefined,
  successText: string,
): void {
  if (error !== undefined && error !== null) {
    const errcode = errorErrcode(error)
    const text = errorMessage(error)
    message.error(errcode === 87012 ? `同一版本不能重复回退（87012）：${text}` : text)
    return
  }
  if (!result) {
    message.error('微信未返回结果，请到日志页查看本次调用详情')
    return
  }
  if (result.ok) {
    message.success(result.message ?? successText)
    return
  }
  const errcode = result.errcode ?? null
  const text = result.message ?? result.errmsg ?? '微信未返回原因'
  message.error(errcode === 87012 ? `同一版本不能重复回退（87012）：${text}` : text)
}

/** 执行无需额外参数的发布类动作并刷新相关数据。 */
async function runAction(
  appid: string,
  action: 'release' | 'revert-gray' | 'revert-previous',
): Promise<void> {
  actionLoading.value = true
  try {
    let error: unknown = null
    let result: components['schemas']['AuditActionResult'] | undefined

    if (action === 'release') {
      const res = await client.POST('/releases/{appid}/release', { params: { path: { appid } } })
      error = res.error
      result = res.data
    } else if (action === 'revert-gray') {
      const res = await client.POST('/releases/{appid}/revert-gray', {
        params: { path: { appid } },
      })
      error = res.error
      result = res.data
    } else {
      const res = await client.POST('/releases/{appid}/revert', {
        params: { path: { appid } },
        body: {},
      })
      error = res.error
      result = res.data
    }

    const successText =
      action === 'release'
        ? '已提交全量发布，线上版本稍后刷新'
        : action === 'revert-gray'
          ? '已取消分阶段发布'
          : '已提交回退到上一个版本'
    handleResult(error, result, successText)
    if (action === 'revert-gray') {
      grayPlan.value = null
      grayPlanLoaded.value = false
    }
    await queryVersion(appid)
    await loadLedger()
  } finally {
    actionLoading.value = false
  }
}

watch(selectedAppid, async (appid) => {
  // 切换小程序时清空上一个的灰度/回退/二维码状态，避免误操作。
  grayPlan.value = null
  grayPlanLoaded.value = false
  historyVersions.value = []
  historyLoaded.value = false
  qrCode.value = null
  version.value = null
  if (appid === null) {
    return
  }
  await queryVersion(appid)
})

watch(ledgerAppid, () => {
  ledgerPage.value = 1
  if (ledgerTimer !== null) {
    window.clearTimeout(ledgerTimer)
  }
  ledgerTimer = window.setTimeout(() => {
    void loadLedger()
  }, 300)
})

onMounted(async () => {
  await loadAuthorizers()
  await loadLedger()
})

onUnmounted(() => {
  if (ledgerTimer !== null) {
    window.clearTimeout(ledgerTimer)
  }
})
</script>

<template>
  <div class="page">
    <page-header
      title="发布管理"
      description="体验版与线上版查询、全量发布、分阶段发布（灰度）、版本回退与发布台账"
    />

    <n-card class="section-card" title="小程序与版本" :bordered="true">
      <div class="page-toolbar">
        <n-select
          v-model:value="selectedAppid"
          class="toolbar-select-wide"
          :options="authorizerOptions"
          placeholder="选择小程序"
          filterable
          clearable
        />
        <n-input
          v-model:value="appidInput"
          class="toolbar-input"
          placeholder="或直接填写 appid"
          clearable
        />
        <n-button :loading="versionLoading" @click="queryVersion()">查询版本</n-button>
      </div>

      <p class="note-text">
        体验版来自上传代码发布后的版本；线上版是用户实际使用的版本。发布与回退都以小程序为单位。
      </p>

      <div v-if="version" class="version-grid">
        <div class="version-cell">
          <div class="version-title">体验版</div>
          <div class="version-line">版本：{{ version.expVersion ?? EMPTY_TEXT }}</div>
          <div class="version-line">描述：{{ version.expDesc ?? EMPTY_TEXT }}</div>
          <div class="version-line">时间：{{ formatDateTime(version.expTime) }}</div>
        </div>
        <div class="version-cell">
          <div class="version-title">线上版</div>
          <div class="version-line">版本：{{ version.releaseVersion ?? EMPTY_TEXT }}</div>
          <div class="version-line">描述：{{ version.releaseDesc ?? EMPTY_TEXT }}</div>
          <div class="version-line">时间：{{ formatDateTime(version.releaseTime) }}</div>
        </div>
      </div>
      <n-empty v-else description="尚未查询版本信息" class="version-empty" />
    </n-card>

    <n-card v-if="selectedAppid" class="section-card" title="发布操作" :bordered="true">
      <div class="action-row">
        <n-button type="primary" :loading="actionLoading" @click="confirmRelease">
          发布线上版本
        </n-button>
        <n-button type="primary" secondary @click="grayVisible = true">分阶段发布</n-button>
        <n-button :loading="actionLoading" @click="loadGrayPlan">查看灰度计划</n-button>
        <n-button type="error" secondary :loading="actionLoading" @click="confirmRevertGray">
          取消分阶段发布
        </n-button>
        <n-button :loading="actionLoading" @click="loadHistoryVersions">可回退版本</n-button>
        <n-button type="error" secondary :loading="actionLoading" @click="confirmRevertPrev">
          回退到上一个版本
        </n-button>
        <n-button @click="openQRCode">体验版二维码</n-button>
      </div>

      <p class="note-text">
        全量发布的是最后一个审核通过的版本且立即生效；灰度比例只能递增，需要缩小时只能取消分阶段发布；
        回退依赖上一个线上版本，无上一个线上版本时无法回退，且同一版本回退一次后不能再回退（87012）。
      </p>

      <div v-if="grayPlanLoaded" class="sub-block">
        <div class="sub-title">灰度计划</div>
        <div v-if="grayPlan" class="plan-grid">
          <div class="plan-line">状态：{{ grayStatusText }}</div>
          <div class="plan-line">
            灰度比例：
            {{ grayPlan.grayPercentage === null || grayPlan.grayPercentage === undefined
              ? EMPTY_TEXT
              : `${grayPlan.grayPercentage}%` }}
          </div>
          <div class="plan-line">创建时间：{{ formatDateTime(grayPlan.createTimestamp) }}</div>
          <div class="plan-line">项目成员优先：{{ grayPlan.supportDebugerFirst ? '是' : '否' }}</div>
          <div class="plan-line">体验成员优先：{{ grayPlan.supportExperiencerFirst ? '是' : '否' }}</div>
        </div>
        <n-empty v-else description="暂无灰度计划" />
      </div>

      <div v-if="historyLoaded" class="sub-block">
        <div class="sub-title">可回退版本</div>
        <p class="note-text">官方最多保留 5 个可回退版本；列表按提交时间由新到旧。</p>
        <n-data-table
          :columns="historyColumns"
          :data="historyVersions"
          :loading="actionLoading"
          :bordered="false"
          :row-key="(row: HistoryVersion) => String(row.appVersion)"
          :scroll-x="800"
          size="small"
        />
      </div>
    </n-card>

    <n-card title="发布台账" :bordered="true">
      <div class="page-toolbar">
        <n-input
          v-model:value="ledgerAppid"
          class="toolbar-input"
          placeholder="按 appid 过滤"
          clearable
        />
        <div class="spacer" />
        <n-button size="small" :loading="ledgerLoading" @click="loadLedger">刷新</n-button>
      </div>
      <n-data-table
        :columns="ledgerColumns"
        :data="ledgerItems"
        :loading="ledgerLoading"
        :bordered="false"
        :row-key="(row: ReleaseRecord) => `${row.appid}-${row.action}-${row.createdAt}`"
        :pagination="ledgerPagination"
        :remote="true"
        :scroll-x="1100"
        size="small"
      />
    </n-card>

    <n-modal v-model:show="grayVisible" preset="card" title="分阶段发布" class="form-modal">
      <div class="form-block">
        <div class="form-label">目标小程序</div>
        <div class="form-text">{{ selectedLabel }}</div>
      </div>
      <div class="form-block">
        <div class="form-label">灰度比例（%）</div>
        <n-input-number
          v-model:value="grayPercentage"
          class="full-width"
          :min="0"
          :max="100"
          :precision="0"
        />
        <p class="note-text">比例只能递增：微信不允许把灰度比例调小，需要缩小时只能取消分阶段发布后重新发起。</p>
      </div>
      <div class="switch-row">
        <span class="form-label">按项目成员优先</span>
        <n-switch v-model:value="supportDebugerFirst" />
      </div>
      <div class="switch-row">
        <span class="form-label">按体验成员优先</span>
        <n-switch v-model:value="supportExperiencerFirst" />
      </div>
      <p class="note-text">分阶段发布的也是最后一个审核通过的版本，只对指定比例的用户生效。</p>
      <template #footer>
        <div class="modal-footer">
          <n-button @click="grayVisible = false">取消</n-button>
          <n-button type="primary" :loading="graySubmitting" @click="submitGray">提交</n-button>
        </div>
      </template>
    </n-modal>

    <n-modal v-model:show="qrVisible" preset="card" title="体验版二维码" class="form-modal">
      <div class="form-block">
        <div class="form-label">目标小程序</div>
        <div class="form-text">{{ selectedLabel }}</div>
      </div>
      <div class="form-block">
        <div class="form-label">扫码后进入的页面（可选）</div>
        <div class="qr-input-row">
          <n-input v-model:value="qrPath" placeholder="例如 pages/index/index" clearable />
          <n-button :loading="qrLoading" @click="loadQRCode">生成二维码</n-button>
        </div>
        <p class="note-text">扫码后进入该路径；留空则进入小程序首页。</p>
      </div>
      <div class="qr-box">
        <img v-if="qrSrc" class="qr-image" :src="qrSrc" alt="体验版二维码" />
        <n-empty v-else description="尚未生成二维码" />
      </div>
      <template #footer>
        <div class="modal-footer">
          <n-button @click="qrVisible = false">关闭</n-button>
        </div>
      </template>
    </n-modal>

    <n-alert v-if="!selectedAppid" class="hint-alert" type="info" show-icon>
      请先在上方选择小程序，选中后才会显示发布、灰度与回退等操作。
    </n-alert>
  </div>
</template>

<style scoped>
.section-card {
  margin-bottom: 12px;
}

.toolbar-input {
  width: 240px;
}

.toolbar-select-wide {
  width: 300px;
}

.note-text {
  margin: 10px 0 0;
  font-size: 14px;
  line-height: 1.6;
  color: #3d4a5c;
}

.version-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 12px;
}

.version-cell {
  flex: 1 1 260px;
  min-width: 220px;
  padding: 10px 12px;
  border: 1px solid #dbe5f1;
  border-radius: 8px;
}

.version-title {
  font-size: 14px;
  font-weight: 600;
  color: #17233d;
  margin-bottom: 6px;
}

.version-line {
  font-size: 14px;
  line-height: 1.7;
  color: #1f2937;
  word-break: break-all;
}

.version-empty {
  margin-top: 12px;
}

.action-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.sub-block {
  margin-top: 14px;
  padding-top: 12px;
  border-top: 1px solid #eef2f8;
}

.sub-title {
  font-size: 14px;
  font-weight: 600;
  color: #17233d;
  margin-bottom: 6px;
}

.plan-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 24px;
}

.plan-line {
  font-size: 14px;
  line-height: 1.7;
  color: #1f2937;
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

.switch-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}

.switch-row .form-label {
  margin-bottom: 0;
}

.full-width {
  width: 100%;
}

.qr-input-row {
  display: flex;
  gap: 8px;
  align-items: center;
}

.qr-box {
  display: flex;
  justify-content: center;
  padding: 8px 0;
}

.qr-image {
  width: 240px;
  height: 240px;
  max-width: 70vw;
  max-height: 70vw;
  image-rendering: pixelated;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.hint-alert {
  margin-top: 6px;
}

:deep(.dialog-lines p) {
  margin: 0 0 6px;
  font-size: 14px;
  line-height: 1.6;
  color: #3d4a5c;
}

/* 弹窗宽度带 max-width: 94vw，窄屏不溢出 */
.form-modal {
  width: 540px;
  max-width: 94vw;
}

@media (max-width: 820px) {
  .toolbar-input,
  .toolbar-select-wide {
    width: 100%;
  }

  .action-row :deep(.n-button) {
    width: 100%;
  }

  .qr-input-row {
    flex-direction: column;
    align-items: stretch;
  }

  .modal-footer :deep(.n-button) {
    flex: 1;
  }
}
</style>
