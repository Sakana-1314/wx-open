<script setup lang="ts">
// 新建批量任务向导（四步）：选择类型 → 选择小程序 → 参数配置 + 预览最终请求体 → 确认提交。
// 设计要点：
//   1. 每次调用都返回类型化的 JobCreateRequest（契约来自 api/openapi.yaml，禁止 any）；
//   2. 第 3 步的「预览最终请求体」是提交前最后一道防线：预览存在无效项时无法进入第 4 步；
//   3. 表单任何字段变化都会让上一次预览结果失效（用请求签名 watcher 实现），避免"预览后再改参数"。
import { computed, h, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NDynamicInput,
  NDynamicTags,
  NInput,
  NInputNumber,
  NModal,
  NRadio,
  NRadioButton,
  NRadioGroup,
  NSelect,
  NStep,
  NSteps,
  NSwitch,
  NTag,
  useMessage,
  type DataTableColumns,
  type SelectOption,
} from 'naive-ui'

import { client, errorErrcode, errorMessage } from '@/api/client'
import type { components } from '@/api/schema'
import PageHeader from '@/components/PageHeader.vue'
import { EMPTY_TEXT, jobTypeText } from '@/utils/format'

type JobType = components['schemas']['JobType']
type AppidSelection = components['schemas']['AppidSelection']
type AuthorizerFilter = components['schemas']['AuthorizerFilter']
type JobCreateRequest = components['schemas']['JobCreateRequest']
type CommitOptions = components['schemas']['CommitOptions']
type AuditOptions = components['schemas']['AuditOptions']
type ReleaseOptions = components['schemas']['ReleaseOptions']
type DomainApplyRequest = components['schemas']['DomainApplyRequest']
type JobPreviewResponse = components['schemas']['JobPreviewResponse']
type JobPreviewItem = components['schemas']['JobPreviewItem']
type CodeTemplate = components['schemas']['CodeTemplate']

/** 版本号 / 描述模式支持的占位符（与后端渲染器一致）。 */
const PLACEHOLDER_HINT = '可用占位符：{{date}} {{time}} {{datetime}} {{appid}} {{nick_name}} {{template_id}} {{seq}}'
/** 默认版本号模式。 */
const DEFAULT_VERSION_PATTERN = 'v{{date}}'
/** 默认描述模式。 */
const DEFAULT_DESC_PATTERN = '批量下发 {{date}}'

/** 选择小程序的方式。 */
type SelectionMode = 'all' | 'filter' | 'manual'

/** 类型卡片模型。 */
interface JobTypeCard {
  value: JobType
  label: string
  desc: string
}

/** 类型说明：一句话用途 + 前置条件（正文色展示，不用灰色小字）。 */
const JOB_TYPE_CARDS: JobTypeCard[] = [
  {
    value: 'commit',
    label: '批量上传代码（commit）',
    desc: '用所选模板为每个小程序上传代码，上传成功即生成体验版。前置条件：模板库中已有普通模板。',
  },
  {
    value: 'submit_audit',
    label: '批量提审（submit_audit）',
    desc: '提交代码审核。前置条件：最近一次上传已完成、隐私检测任务已结束（否则 61039），类目已在小程序侧配置（否则 85008）。',
  },
  {
    value: 'release',
    label: '批量发布（release）',
    desc: '把已审核通过的版本发布上线。可选全量发布，或 0-100 分阶段灰度（比例只能递增）。',
  },
  {
    value: 'pipeline',
    label: '全流程（pipeline）',
    desc: '会按「上传 → 等隐私检测 → 提审 → 等审核通过 → 发布」串行推进，每一步都要等前一步真正完成，可随时暂停。',
  },
  {
    value: 'sync_audit_status',
    label: '对账审核状态（sync_audit_status）',
    desc: '拉取微信侧最新审核状态回写本地，用于修正漏收的事件通知；不改动代码，随时可执行。',
  },
  {
    value: 'undo_audit',
    label: '批量撤回（undo_audit）',
    desc: '撤回处于「审核中」的提审单，撤回后可修改代码重新提交。前置条件：存在审核中的提审单。',
  },
  {
    value: 'speedup_audit',
    label: '批量加急（speedup_audit）',
    desc: '对审核中的提审单使用加急额度。额度是服务商级、旗下小程序共用，用尽返回 89405 并暂停作业。',
  },
  {
    value: 'revert',
    label: '批量回退（revert）',
    desc: '把线上版本回退到上一版本（微信只保留上一版本）。回退后如需再次前进必须重新提审。',
  },
  {
    value: 'toggle_visit',
    label: '批量服务状态（toggle_visit）',
    desc: '查询/设置小程序的服务状态（是否可被搜索与访问）。前置条件：小程序已发布上线，否则返回 85015。',
  },
  {
    value: 'set_domain',
    label: '批量域名配置（set_domain）',
    desc: '配置服务器域名与业务域名。前置条件：需先发布上线才生效；授权托管后只能用第三方平台登记的域名。',
  },
  {
    value: 'sync_info',
    label: '同步授权方信息（sync_info）',
    desc: '同步昵称、头像、主体、权限集等授权方资料。不调用代码管理接口，建议下发前先执行一次以保证信息最新。',
  },
]

const router = useRouter()
const message = useMessage()

/** 当前步骤索引（0-3），展示时 +1 交给 n-steps。 */
const step = ref(0)

// ---- 第 1 步：类型 ----
const jobType = ref<JobType>('commit')

// ---- 第 2 步：目标小程序 ----
const selectionMode = ref<SelectionMode>('all')
const filterGroupName = ref('')
const filterTags = ref<string[]>([])
const onlyDevPermission = ref(false)
const selectedAppids = ref<string[]>([])
const authorizerOptions = ref<SelectOption[]>([])
const authorizerLoading = ref(false)
/** 当前选择命中的小程序数量（手动勾选=选中数；其余模式取 /authorizers 的 total）。 */
const matchedCount = ref(0)
const countLoading = ref(false)

// ---- 第 3 步：参数 ----
const templates = ref<CodeTemplate[]>([])
const templatesLoading = ref(false)
const commitTemplateId = ref<number | null>(null)
const userVersionPattern = ref(DEFAULT_VERSION_PATTERN)
const userDescPattern = ref(DEFAULT_DESC_PATTERN)
const extTemplate = ref('')
const extOverrides = ref<Array<{ key: string; value: string }>>([])

const auditProfileId = ref<number | null>(null)
const versionDescPattern = ref('批量提审 {{date}}')
const privacyApiNotUse = ref(false)
const orderPath = ref('')

const grayPercentage = ref<number | null>(null)
const supportDebugerFirst = ref(false)
const supportExperiencerFirst = ref(false)

const domainMode = ref<'registered' | 'direct'>('registered')
const domainAction = ref<'add' | 'delete' | 'set' | 'get'>('set')
const serverDomainsText = ref('')
const businessDomainsText = ref('')

const concurrency = ref(3)
const dryRun = ref(false)
const note = ref('')

// ---- 预览 / 提交 ----
const showPreview = ref(false)
const previewLoading = ref(false)
const previewResult = ref<JobPreviewResponse | null>(null)
const previewError = ref<string | null>(null)
const creating = ref(false)
const createError = ref<string | null>(null)

/** 是否需要「上传代码」相关参数。 */
const needsCommit = computed(() => jobType.value === 'commit' || jobType.value === 'pipeline')
/** 是否需要「提审」相关参数。 */
const needsAudit = computed(() => jobType.value === 'submit_audit' || jobType.value === 'pipeline')
/** 是否需要「发布」相关参数。 */
const needsRelease = computed(() => jobType.value === 'release' || jobType.value === 'pipeline')
/** 是否需要域名配置参数。 */
const needsDomain = computed(() => jobType.value === 'set_domain')

/** 类型下拉/摘要用文案。 */
const typeText = computed(() => jobTypeText(jobType.value))

/** 目标范围文案。 */
const selectionLabel = computed(() => {
  if (selectionMode.value === 'manual') {
    return `手动勾选 ${selectedAppids.value.length} 个`
  }
  if (selectionMode.value === 'filter') {
    const parts: string[] = []
    if (filterGroupName.value.trim() !== '') {
      parts.push(`分组 ${filterGroupName.value.trim()}`)
    }
    if (filterTags.value.length > 0) {
      parts.push(`标签 ${filterTags.value.join(' / ')}`)
    }
    if (onlyDevPermission.value) {
      parts.push('仅已授权开发权限集')
    }
    return parts.length > 0 ? `按条件筛选：${parts.join('，')}` : '按条件筛选（未填条件=全部）'
  }
  return '全部已授权且启用'
})

/** 模板下拉选项。 */
const templateOptions = computed<SelectOption[]>(() =>
  templates.value.map((item) => ({
    label: `${item.templateId} · ${item.userVersion ?? '无版本号'}${item.isDefault ? '（默认）' : ''}${
      item.note ? ` · ${item.note}` : ''
    }`,
    value: item.templateId,
  })),
)

/** 提审配置：commit 型且未选模板时无法预览。 */
const commitReady = computed(() => !needsCommit.value || commitTemplateId.value !== null)

/** 预览是否通过（提交前置条件）。 */
const previewPassed = computed(() => previewResult.value !== null && previewResult.value.invalidCount === 0)

/** 「下一步」是否禁用：第 2 步无目标、第 3 步未通过预览时禁用。 */
const nextDisabled = computed(() => {
  if (step.value === 1) {
    return matchedCount.value <= 0
  }
  if (step.value === 2) {
    return !commitReady.value || !previewPassed.value
  }
  return false
})

/** 预览结果表格列。 */
const previewColumns: DataTableColumns<JobPreviewItem> = [
  {
    title: '小程序',
    key: 'appid',
    width: 220,
    render: (row) =>
      h('div', { class: 'cell-stack' }, [
        h('div', { class: 'cell-main' }, row.appid),
        h('div', { class: 'cell-sub' }, row.nickName ?? EMPTY_TEXT),
      ]),
  },
  {
    title: '校验',
    key: 'valid',
    width: 90,
    render: (row) =>
      h(
        NTag,
        { type: row.valid ? 'success' : 'error', size: 'small', round: true, bordered: false },
        { default: () => (row.valid ? '有效' : '无效') },
      ),
  },
  { title: '接口路径', key: 'endpoint', width: 170, render: (row) => row.endpoint },
  {
    title: '问题',
    key: 'problems',
    width: 260,
    render: (row) =>
      row.problems.length === 0
        ? EMPTY_TEXT
        : h(
            'div',
            { class: 'problem-list' },
            row.problems.map((problem) => h('div', { class: 'problem-item' }, problem)),
          ),
  },
  {
    title: '最终请求体',
    key: 'payload',
    width: 380,
    render: (row) => h('pre', { class: 'json-block' }, JSON.stringify(row.payload ?? {}, null, 2)),
  },
]

/** 错误文案：微信侧失败带上 errcode 与中文说明。 */
function describeError(err: unknown): string {
  const code = errorErrcode(err)
  const text = errorMessage(err)
  return code === null ? text : `${text}（微信 errcode ${code}）`
}

/** 多行/逗号分隔文本 → 字符串数组。 */
function splitLines(value: string): string[] {
  return value
    .split(/[\n,，;；]+/)
    .map((item) => item.trim())
    .filter((item) => item !== '')
}

/** 键值对行 → 覆盖表（忽略空键）。 */
function toOverrideMap(rows: Array<{ key: string; value: string }>): Record<string, string> {
  const map: Record<string, string> = {}
  for (const row of rows) {
    const key = row.key.trim()
    if (key !== '') {
      map[key] = row.value
    }
  }
  return map
}

/** n-dynamic-input 新增行的初始值。 */
function createOverrideRow(): { key: string; value: string } {
  return { key: '', value: '' }
}

/** 目标小程序选择（appids 显式指定 / filter 筛选 / 空对象=全部已授权且启用）。 */
const selection = computed<AppidSelection>(() => {
  if (selectionMode.value === 'manual') {
    return { appids: selectedAppids.value }
  }
  if (selectionMode.value === 'filter') {
    const filter: AuthorizerFilter = {}
    const groupName = filterGroupName.value.trim()
    if (groupName !== '') {
      filter.groupName = groupName
    }
    if (filterTags.value.length > 0) {
      filter.tags = [...filterTags.value]
    }
    if (onlyDevPermission.value) {
      filter.onlyDevPermission = true
    }
    return { filter }
  }
  return {}
})

/** 组装最终请求体（与第 3 步表单一一对应）。 */
function buildRequest(): JobCreateRequest {
  const request: JobCreateRequest = {
    type: jobType.value,
    selection: selection.value,
    concurrency: concurrency.value,
    dryRun: dryRun.value,
    note: note.value.trim() === '' ? null : note.value.trim(),
  }

  if (needsCommit.value) {
    const commit: CommitOptions = {
      templateId: commitTemplateId.value ?? 0,
      userVersionPattern: userVersionPattern.value.trim(),
      userDescPattern: userDescPattern.value.trim(),
      extTemplate: extTemplate.value.trim() === '' ? null : extTemplate.value,
      extOverrides: null,
    }
    const overrideMap = toOverrideMap(extOverrides.value)
    if (Object.keys(overrideMap).length > 0) {
      commit.extOverrides = overrideMap
    }
    request.commit = commit
  }

  if (needsAudit.value) {
    const audit: AuditOptions = {
      auditProfileId: auditProfileId.value,
      versionDescPattern: versionDescPattern.value.trim(),
      privacyApiNotUse: privacyApiNotUse.value,
      orderPath: orderPath.value.trim() === '' ? null : orderPath.value.trim(),
    }
    request.audit = audit
  }

  if (needsRelease.value) {
    const release: ReleaseOptions = {
      grayPercentage: grayPercentage.value,
      supportDebugerFirst: supportDebugerFirst.value,
      supportExperiencerFirst: supportExperiencerFirst.value,
    }
    request.release = release
  }

  if (needsDomain.value) {
    const serverDomains = splitLines(serverDomainsText.value)
    const businessDomains = splitLines(businessDomainsText.value)
    const domain: DomainApplyRequest = {
      selection: selection.value,
      mode: domainMode.value,
      action: domainAction.value,
    }
    if (serverDomains.length > 0) {
      domain.serverDomain = { requestDomain: serverDomains }
    }
    if (businessDomains.length > 0) {
      domain.businessDomain = businessDomains
    }
    request.domain = domain
  }

  return request
}

/** 请求签名：任何字段变化都会让上一次预览结果失效。 */
const requestSignature = computed(() => JSON.stringify(buildRequest()))

watch(requestSignature, () => {
  previewResult.value = null
  previewError.value = null
})

/** 第 4 步摘要。 */
const summaryItems = computed<Array<{ label: string; value: string }>>(() => {
  const items: Array<{ label: string; value: string }> = [
    { label: '作业类型', value: typeText.value },
    { label: '目标范围', value: selectionLabel.value },
    { label: '目标数量', value: `${matchedCount.value} 个小程序` },
    { label: '并发数', value: `${concurrency.value}（同一小程序始终串行）` },
    {
      label: '执行方式',
      value: dryRun.value ? '试运行：只预览不调用微信' : '真实执行：会真实调用微信接口',
    },
  ]
  if (note.value.trim() !== '') {
    items.push({ label: '作业备注', value: note.value.trim() })
  }
  if (needsCommit.value) {
    items.push({
      label: '代码模板',
      value: commitTemplateId.value === null ? '未选择' : String(commitTemplateId.value),
    })
    items.push({ label: '版本号模式', value: userVersionPattern.value.trim() })
    items.push({ label: '版本描述模式', value: userDescPattern.value.trim() })
    items.push({
      label: 'ext_json 模式',
      value:
        extTemplate.value.trim() === ''
          ? '简单模式：自动生成 {extAppid, ext:{…变量}}'
          : '高级模式：使用自定义 ext_json 模板',
    })
    const overrideCount = Object.keys(toOverrideMap(extOverrides.value)).length
    items.push({ label: 'ext 变量覆盖', value: overrideCount === 0 ? '无' : `${overrideCount} 个` })
  }
  if (needsAudit.value) {
    items.push({
      label: '提审配置 ID',
      value: auditProfileId.value === null ? '使用默认配置' : String(auditProfileId.value),
    })
    items.push({ label: '版本说明模式', value: versionDescPattern.value.trim() })
    items.push({ label: '声明不使用隐私接口', value: privacyApiNotUse.value ? '是' : '否' })
    items.push({ label: '订单路径', value: orderPath.value.trim() === '' ? '未填写' : orderPath.value.trim() })
  }
  if (needsRelease.value) {
    items.push({
      label: '发布方式',
      value:
        grayPercentage.value === null
          ? '全量发布（立即生效）'
          : `分阶段灰度 ${grayPercentage.value}%`,
    })
    items.push({ label: '优先体验者', value: supportExperiencerFirst.value ? '是' : '否' })
    items.push({ label: '优先开发者', value: supportDebugerFirst.value ? '是' : '否' })
  }
  if (needsDomain.value) {
    items.push({ label: '域名方式', value: domainMode.value === 'registered' ? '先登记再配置' : '直接配置' })
    items.push({ label: '域名操作', value: domainAction.value })
    items.push({ label: '服务器域名', value: splitLines(serverDomainsText.value).join('，') || '未填写' })
    items.push({ label: '业务域名', value: splitLines(businessDomainsText.value).join('，') || '未填写' })
  }
  return items
})

/** 加载手动勾选用的授权方选项（最多 200 条）。 */
async function loadAuthorizerOptions(): Promise<void> {
  authorizerLoading.value = true
  try {
    const res = await client.GET('/authorizers', { params: { query: { page: 1, pageSize: 200 } } })
    if (res.error || !res.data) {
      message.error(errorMessage(res.error))
      return
    }
    authorizerOptions.value = res.data.items.map((item) => ({
      label: item.nickName ? `${item.nickName}（${item.appid}）` : item.appid,
      value: item.appid,
    }))
  } finally {
    authorizerLoading.value = false
  }
}

/** 加载普通模板（templateType === 0），默认选中「默认模板」。 */
async function loadTemplates(): Promise<void> {
  templatesLoading.value = true
  try {
    const res = await client.GET('/templates', { params: { query: { templateType: 0 } } })
    if (res.error || !res.data) {
      message.error(errorMessage(res.error))
      return
    }
    templates.value = res.data.items.filter((item) => item.templateType === 0)
    if (commitTemplateId.value === null) {
      const preferred = templates.value.find((item) => item.isDefault) ?? templates.value[0]
      commitTemplateId.value = preferred ? preferred.templateId : null
    }
  } finally {
    templatesLoading.value = false
  }
}

/** 统计当前选择命中的小程序数量（筛选模式调用一次 /authorizers 取 total）。 */
async function loadMatchedCount(): Promise<void> {
  if (selectionMode.value === 'manual') {
    matchedCount.value = selectedAppids.value.length
    return
  }
  countLoading.value = true
  try {
    const res =
      selectionMode.value === 'all'
        ? await client.GET('/authorizers', { params: { query: { page: 1, pageSize: 1, status: 'authorized' } } })
        : await client.GET('/authorizers', {
            params: {
              query: {
                page: 1,
                pageSize: 1,
                groupName: filterGroupName.value.trim() === '' ? undefined : filterGroupName.value.trim(),
                tag: filterTags.value.length === 1 ? filterTags.value[0] : undefined,
                hasDevPermission: onlyDevPermission.value ? true : undefined,
              },
            },
          })
    if (res.error || !res.data) {
      matchedCount.value = 0
      message.error(errorMessage(res.error))
      return
    }
    matchedCount.value = res.data.total
  } finally {
    countLoading.value = false
  }
}

watch(selectionMode, () => {
  void loadMatchedCount()
})

watch(selectedAppids, () => {
  if (selectionMode.value === 'manual') {
    matchedCount.value = selectedAppids.value.length
  }
})

/** 调 POST /jobs/preview 预览最终请求体。 */
async function runPreview(): Promise<void> {
  previewLoading.value = true
  previewError.value = null
  try {
    const res = await client.POST('/jobs/preview', { body: buildRequest() })
    if (res.error || !res.data) {
      previewResult.value = null
      previewError.value = describeError(res.error)
      showPreview.value = true
      return
    }
    previewResult.value = res.data
    showPreview.value = true
  } finally {
    previewLoading.value = false
  }
}

/** 上一步。 */
function goPrev(): void {
  if (step.value > 0) {
    step.value -= 1
  }
}

/** 下一步（受 nextDisabled 约束）。 */
function goNext(): void {
  if (nextDisabled.value || step.value >= 3) {
    return
  }
  step.value += 1
}

/** 创建任务；409（在途冲突）/422（校验失败）时展示后端 message。 */
async function createJob(): Promise<void> {
  creating.value = true
  createError.value = null
  try {
    const res = await client.POST('/jobs', { body: buildRequest() })
    if (res.error || !res.data) {
      createError.value = describeError(res.error)
      return
    }
    message.success('批量任务已创建')
    await router.push({ name: 'job-detail', params: { id: res.data.id } })
  } finally {
    creating.value = false
  }
}

onMounted(async () => {
  await Promise.all([loadAuthorizerOptions(), loadTemplates(), loadMatchedCount()])
})
</script>

<template>
  <div class="page">
    <page-header title="新建批量任务" description="四步向导：选类型 → 选小程序 → 配参数并预览 → 确认提交" />

    <n-card :bordered="true" class="section-card">
      <n-steps :current="step + 1" size="small">
        <n-step title="选择类型" description="批量做什么" />
        <n-step title="选择小程序" description="目标范围" />
        <n-step title="参数与预览" description="按类型配置" />
        <n-step title="确认提交" description="核对后创建" />
      </n-steps>
    </n-card>

    <!-- 第 1 步：类型 -->
    <n-card v-if="step === 0" :bordered="true" class="section-card" title="第 1 步：选择作业类型">
      <n-radio-group v-model:value="jobType" class="type-grid">
        <n-radio v-for="item in JOB_TYPE_CARDS" :key="item.value" :value="item.value" class="type-card">
          <div class="type-card-body">
            <div class="type-card-title">{{ item.label }}</div>
            <div class="type-card-desc">{{ item.desc }}</div>
          </div>
        </n-radio>
      </n-radio-group>
    </n-card>

    <!-- 第 2 步：目标小程序 -->
    <n-card v-else-if="step === 1" :bordered="true" class="section-card" title="第 2 步：选择小程序">
      <n-radio-group v-model:value="selectionMode" class="mode-group">
        <n-radio-button value="all">全部已授权且启用</n-radio-button>
        <n-radio-button value="filter">按分组 / 标签筛选</n-radio-button>
        <n-radio-button value="manual">手动勾选</n-radio-button>
      </n-radio-group>

      <div v-if="selectionMode === 'filter'" class="filter-panel">
        <div class="field">
          <div class="field-label">分组名</div>
          <n-input v-model:value="filterGroupName" placeholder="留空表示不限分组" />
        </div>
        <div class="field">
          <div class="field-label">标签</div>
          <n-dynamic-tags v-model:value="filterTags" />
          <div class="field-hint">可添加多个标签，命中任一标签即可</div>
        </div>
        <div class="switch-field">
          <span class="field-label">仅选已授权开发权限集的小程序</span>
          <n-switch v-model:value="onlyDevPermission" />
        </div>
        <div class="field-hint">权限集 18（小程序开发与数据分析）是代码管理的前提，未授权的会被微信拒绝。</div>
      </div>

      <div v-else-if="selectionMode === 'manual'" class="filter-panel">
        <div class="field">
          <div class="field-label">选择小程序（可搜索多选）</div>
          <n-select
            v-model:value="selectedAppids"
            multiple
            filterable
            clearable
            :options="authorizerOptions"
            :loading="authorizerLoading"
            placeholder="输入昵称或 appid 搜索"
          />
        </div>
      </div>

      <div v-else class="filter-panel">
        <div class="field-hint">不传 appids 与 filter 时，后端按「全部已授权且启用」的小程序执行。</div>
      </div>

      <div class="count-row">
        <span class="count-text">当前匹配数量：{{ matchedCount }} 个小程序</span>
        <n-button secondary :loading="countLoading" @click="loadMatchedCount">刷新匹配数量</n-button>
      </div>

      <n-alert v-if="matchedCount <= 0" type="warning" show-icon class="section-gap">
        当前没有匹配到小程序，无法进入下一步；请调整筛选条件或先在「小程序管理」完成授权。
      </n-alert>
    </n-card>

    <!-- 第 3 步：参数 + 预览 -->
    <n-card v-else-if="step === 2" :bordered="true" class="section-card" title="第 3 步：参数配置与预览">
      <!-- 上传代码参数 -->
      <div v-if="needsCommit" class="param-block">
        <div class="param-title">上传代码参数</div>
        <div class="field-grid">
          <div class="field">
            <div class="field-label">代码模板（仅普通模板可用）</div>
            <n-select
              v-model:value="commitTemplateId"
              :options="templateOptions"
              :loading="templatesLoading"
              placeholder="选择模板库中的普通模板"
              clearable
            />
          </div>
          <div class="field">
            <div class="field-label">版本号模式</div>
            <n-input v-model:value="userVersionPattern" :placeholder="DEFAULT_VERSION_PATTERN" />
            <div class="field-hint">{{ PLACEHOLDER_HINT }}；渲染后须 ≤64 字符</div>
          </div>
          <div class="field">
            <div class="field-label">版本描述模式</div>
            <n-input v-model:value="userDescPattern" :placeholder="DEFAULT_DESC_PATTERN" />
            <div class="field-hint">{{ PLACEHOLDER_HINT }}</div>
          </div>
        </div>

        <div class="field">
          <div class="field-label">ext_json 模板（留空即「简单模式」）</div>
          <n-input
            v-model:value="extTemplate"
            type="textarea"
            :rows="4"
            placeholder='留空时自动生成 {"extAppid":"…","ext":{…变量}}'
          />
          <div class="field-hint">
            留空为简单模式：自动生成 <code>{extAppid, ext:{…变量}}</code>，小程序用
            <code>wx.getExtConfigSync()</code> 读取；填写则为高级模式，必须是字符串化的 JSON。
          </div>
        </div>

        <div class="field">
          <div class="field-label">ext 变量覆盖（会覆盖小程序自身配置的同名变量）</div>
          <n-dynamic-input
            v-model:value="extOverrides"
            preset="pair"
            key-placeholder="变量名，如 apiBase"
            value-placeholder="变量值"
            :on-create="createOverrideRow"
          />
          <div class="field-hint">键与值都按字符串处理；键留空的行会被忽略。</div>
        </div>
      </div>

      <!-- 提审参数 -->
      <div v-if="needsAudit" class="param-block">
        <div class="param-title">提审参数</div>
        <div class="field-grid">
          <div class="field">
            <div class="field-label">提审配置 ID（可留空使用默认）</div>
            <n-input-number v-model:value="auditProfileId" :min="1" placeholder="留空=默认配置" clearable />
          </div>
          <div class="field">
            <div class="field-label">版本说明模式</div>
            <n-input v-model:value="versionDescPattern" placeholder="批量提审 {{date}}" />
            <div class="field-hint">{{ PLACEHOLDER_HINT }}</div>
          </div>
          <div class="field">
            <div class="field-label">订单路径 orderPath（可选）</div>
            <n-input v-model:value="orderPath" placeholder="如 pages/order/index" />
          </div>
        </div>
        <div class="switch-field">
          <span class="field-label">声明不使用检测出的隐私接口</span>
          <n-switch v-model:value="privacyApiNotUse" />
        </div>
        <div class="field-hint">
          打开后提交审核时按「不使用隐私接口」声明；若小程序实际调用了隐私接口，会被微信驳回。
        </div>
      </div>

      <!-- 发布参数 -->
      <div v-if="needsRelease" class="param-block">
        <div class="param-title">发布参数</div>
        <div class="field-grid">
          <div class="field">
            <div class="field-label">灰度比例（0-100，留空=全量发布）</div>
            <n-input-number
              v-model:value="grayPercentage"
              :min="0"
              :max="100"
              placeholder="留空=全量发布"
              clearable
            />
            <div class="field-hint">
              不填即全量发布、立即生效；填 0-100 表示分阶段发布，且比例只能递增（微信侧限制）。
            </div>
          </div>
          <div class="switch-field">
            <span class="field-label">优先开发者体验</span>
            <n-switch v-model:value="supportDebugerFirst" />
          </div>
          <div class="switch-field">
            <span class="field-label">优先体验者体验</span>
            <n-switch v-model:value="supportExperiencerFirst" />
          </div>
        </div>
      </div>

      <!-- 域名参数 -->
      <div v-if="needsDomain" class="param-block">
        <div class="param-title">域名配置参数</div>
        <div class="field-grid">
          <div class="field">
            <div class="field-label">配置方式</div>
            <n-select
              v-model:value="domainMode"
              :options="[
                { label: '先登记到第三方平台再配置（推荐）', value: 'registered' },
                { label: '直接配置（需发布上线后生效）', value: 'direct' },
              ]"
            />
          </div>
          <div class="field">
            <div class="field-label">操作类型</div>
            <n-select
              v-model:value="domainAction"
              :options="[
                { label: 'set 覆盖设置', value: 'set' },
                { label: 'add 追加', value: 'add' },
                { label: 'delete 删除', value: 'delete' },
                { label: 'get 查询', value: 'get' },
              ]"
            />
          </div>
        </div>
        <div class="field-grid">
          <div class="field">
            <div class="field-label">服务器域名（requestDomain，每行一个）</div>
            <n-input v-model:value="serverDomainsText" type="textarea" :rows="3" placeholder="https://api.example.com" />
          </div>
          <div class="field">
            <div class="field-label">业务域名（每行一个）</div>
            <n-input v-model:value="businessDomainsText" type="textarea" :rows="3" placeholder="https://m.example.com" />
          </div>
        </div>
        <div class="field-hint">域名须为 https 且已完成校验；托管后小程序只能使用第三方平台登记的域名。</div>
      </div>

      <!-- 通用参数 -->
      <div class="param-block">
        <div class="param-title">通用参数</div>
        <div class="field-grid">
          <div class="field">
            <div class="field-label">并发数（1-8）</div>
            <n-input-number v-model:value="concurrency" :min="1" :max="8" />
            <div class="field-hint">同一小程序始终串行、逐个小程序并发，避免触发微信 9402202。</div>
          </div>
          <div class="field">
            <div class="field-label">作业备注（可选）</div>
            <n-input v-model:value="note" placeholder="例如：3 月活动批量发布" />
          </div>
          <div class="switch-field">
            <span class="field-label">试运行（只预览不调用微信）</span>
            <n-switch v-model:value="dryRun" />
          </div>
        </div>
      </div>

      <div class="step-footer">
        <n-button type="primary" :loading="previewLoading" :disabled="!commitReady" @click="runPreview">
          预览最终请求体
        </n-button>
        <span class="count-text">提交前必须预览通过（无无效项）才能进入第 4 步</span>
      </div>

      <n-alert v-if="!commitReady" type="error" show-icon class="section-gap">
        请先选择代码模板：批量上传与全流程必须指定模板库中的普通模板。
      </n-alert>
      <n-alert v-else-if="previewResult === null && previewError === null" type="info" show-icon class="section-gap">
        尚未预览：请点击「预览最终请求体」核对每个小程序的 endpoint 与 payload。
      </n-alert>
      <n-alert v-else-if="previewError !== null" type="error" show-icon class="section-gap">
        预览失败：{{ previewError }}
      </n-alert>
      <n-alert
        v-else-if="previewResult && previewResult.invalidCount > 0"
        type="error"
        show-icon
        class="section-gap"
      >
        预览发现 {{ previewResult.invalidCount }} 个无效项，无法进入提交；请按 problems 修正后重新预览。
      </n-alert>
      <n-alert v-else-if="previewResult" type="success" show-icon class="section-gap">
        预览通过：{{ previewResult.validCount }} 个小程序可下发，可以进入第 4 步。
      </n-alert>
    </n-card>

    <!-- 第 4 步：确认并提交 -->
    <n-card v-else :bordered="true" class="section-card" title="第 4 步：确认并提交">
      <n-alert :type="dryRun ? 'warning' : 'error'" show-icon class="section-gap">
        {{
          dryRun
            ? '当前为「试运行」：只生成请求体与预览，不会真实调用微信接口。'
            : '当前为「真实执行」：创建后会真实调用微信接口上传/提审/发布，请确认目标小程序数量与参数无误。'
        }}
      </n-alert>

      <template v-if="needsRelease && grayPercentage === null">
        <n-alert type="warning" show-icon class="section-gap">
          灰度比例留空 = 全量发布且立即生效；如需分阶段，请返回第 3 步填写 0-100（比例只能递增）。
        </n-alert>
      </template>

      <div class="summary">
        <div v-for="item in summaryItems" :key="item.label" class="summary-row">
          <div class="summary-label">{{ item.label }}</div>
          <div class="summary-value">{{ item.value }}</div>
        </div>
      </div>

      <n-alert v-if="createError !== null" type="error" show-icon class="section-gap">{{ createError }}</n-alert>
    </n-card>

    <div class="wizard-actions">
      <n-button v-if="step > 0" secondary @click="goPrev">上一步</n-button>
      <n-button v-if="step < 3" type="primary" :disabled="nextDisabled" @click="goNext">下一步</n-button>
      <n-button v-else type="primary" :loading="creating" @click="createJob">创建任务</n-button>
    </div>

    <!-- 预览弹窗 -->
    <n-modal
      v-model:show="showPreview"
      preset="card"
      title="预览最终请求体"
      class="preview-modal"
      :style="{ maxWidth: '94vw', width: '980px' }"
    >
      <n-alert v-if="previewError !== null" type="error" show-icon>{{ previewError }}</n-alert>
      <template v-else-if="previewResult">
        <div class="preview-summary">
          <n-tag type="success" size="small" round :bordered="false">有效 {{ previewResult.validCount }}</n-tag>
          <n-tag
            :type="previewResult.invalidCount > 0 ? 'error' : 'default'"
            size="small"
            round
            :bordered="false"
          >
            无效 {{ previewResult.invalidCount }}
          </n-tag>
          <span class="count-text">共 {{ previewResult.items.length }} 个小程序</span>
        </div>
        <n-alert v-if="previewResult.invalidCount > 0" type="error" show-icon class="section-gap">
          存在无效项时无法进入提交，请先按「问题」列修正参数（例如缺少模板、缺开发权限、类目未配置）。
        </n-alert>
        <n-data-table
          :columns="previewColumns"
          :data="previewResult.items"
          :bordered="false"
          :scroll-x="1120"
          :row-key="(row: JobPreviewItem) => row.appid"
          size="small"
        />
      </template>
      <template #footer>
        <div class="modal-footer">
          <n-button secondary @click="showPreview = false">关闭</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.section-card {
  margin-bottom: 12px;
}

.section-gap {
  margin-top: 12px;
}

.type-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 12px;
}

.type-card {
  align-items: flex-start;
  border: 1px solid #dbe5f1;
  border-radius: 8px;
  padding: 12px;
  height: 100%;
}

.type-card-body {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.type-card-title {
  font-size: 14px;
  font-weight: 600;
  color: #17233d;
}

.type-card-desc {
  font-size: 14px;
  line-height: 1.5;
  color: #3d4a5c;
  white-space: normal;
}

.mode-group {
  margin-bottom: 12px;
}

.filter-panel {
  margin-bottom: 12px;
}

.field,
.switch-field {
  margin-bottom: 12px;
}

.field-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 4px 16px;
}

.field-label {
  font-size: 14px;
  font-weight: 600;
  color: #3d4a5c;
  margin-bottom: 6px;
}

.field-hint {
  font-size: 14px;
  line-height: 1.5;
  color: #3d4a5c;
  margin-top: 6px;
}

.switch-field {
  display: flex;
  align-items: center;
  gap: 8px;
}

.switch-field .field-label {
  margin-bottom: 0;
}

.param-block {
  padding: 12px 0;
  border-bottom: 1px solid #eef2f8;
}

.param-block:last-of-type {
  border-bottom: none;
}

.param-title {
  font-size: 14px;
  font-weight: 600;
  color: #17233d;
  margin-bottom: 10px;
}

.count-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
}

.count-text {
  font-size: 14px;
  color: #1f2937;
}

.step-footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  padding-top: 12px;
}

.summary {
  border: 1px solid #eef2f8;
  border-radius: 8px;
  padding: 4px 12px;
}

.summary-row {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 8px 0;
  border-bottom: 1px solid #eef2f8;
}

.summary-row:last-child {
  border-bottom: none;
}

.summary-label {
  width: 150px;
  flex-shrink: 0;
  font-size: 14px;
  font-weight: 600;
  color: #3d4a5c;
}

.summary-value {
  flex: 1;
  min-width: 0;
  font-size: 14px;
  color: #1f2937;
  word-break: break-all;
}

.wizard-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-bottom: 12px;
}

.preview-summary {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  margin-bottom: 12px;
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

.problem-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.problem-item {
  font-size: 14px;
  color: #1f2937;
}

.json-block {
  margin: 0;
  max-height: 220px;
  overflow: auto;
  font-size: 13px;
  line-height: 1.5;
  color: #1f2937;
  background: #f7f9fc;
  border-radius: 6px;
  padding: 8px;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

@media (max-width: 820px) {
  .type-grid {
    grid-template-columns: 1fr;
  }

  .field-grid {
    grid-template-columns: 1fr;
  }

  .summary-label {
    width: 100%;
  }

  .summary-row {
    flex-direction: column;
    gap: 4px;
  }

  /* 窄屏工具条与向导按钮占满整行 */
  .wizard-actions :deep(.n-button),
  .step-footer :deep(.n-button),
  .count-row :deep(.n-button) {
    width: 100%;
  }

  .modal-footer :deep(.n-button) {
    width: 100%;
  }
}
</style>
