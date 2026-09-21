<script setup lang="ts">
// 小程序管理页（/authorizers 与 /authorizers/:appid 共用）：
//   1. 顶部工具条筛选（关键字/授权状态/分组/标签/仅看缺开发权限集）+ 分页 + 多选；
//   2. page-header 操作区：生成授权链接、同步选中、重新拉取令牌、同步全部；
//   3. 详情抽屉：全量资料 + 同步信息/刷新令牌/体验版二维码/版本信息/服务状态/页面列表；
//   4. 编辑弹窗：备注/分组/标签/启停/ext 变量/提审配置，删除按钮置于弹窗底部左侧（二次确认后 DELETE）。
import { computed, h, onMounted, reactive, ref, watch, type VNode } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  NAvatar,
  NButton,
  NCard,
  NDataTable,
  NDrawer,
  NDrawerContent,
  NDynamicTags,
  NEmpty,
  NForm,
  NFormItem,
  NIcon,
  NImage,
  NInput,
  NInputNumber,
  NModal,
  NPagination,
  NQrCode,
  NSelect,
  NSpin,
  NSwitch,
  NTag,
  useDialog,
  useMessage,
  type DataTableColumns,
  type DataTableRowKey,
  type SelectOption,
} from 'naive-ui'
import { AddOutline, CopyOutline, RefreshOutline, SyncOutline } from '@vicons/ionicons5'

import { client, errorMessage } from '@/api/client'
import PageHeader from '@/components/PageHeader.vue'
import StatusTag from '@/components/StatusTag.vue'
import type { components } from '@/api/schema'
import { EMPTY_TEXT, formatDateTime, formatRelativeTime, truncateMiddle } from '@/utils/format'
import { useIsMobile } from '@/utils/media'

type Authorizer = components['schemas']['Authorizer']
type AuthorizerDetail = components['schemas']['AuthorizerDetail']
type AuthorizerUpdateRequest = components['schemas']['AuthorizerUpdateRequest']
type AuthorizationStatus = components['schemas']['AuthorizationStatus']
type AuthType = components['schemas']['AuthType']
type AuthorizationUrlResponse = components['schemas']['AuthorizationUrlResponse']
type DomainSnapshot = components['schemas']['DomainSnapshot']
type SyncSummary = components['schemas']['SyncSummary']
type VersionInfo = components['schemas']['VersionInfo']
type VisitStatus = components['schemas']['VisitStatus']
/** 代码来源：模板库下发 / CI 直传（契约里是字符串枚举，下拉绑定用该别名表达）。 */
type CodeSource = 'template' | 'direct_commit'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const dialog = useDialog()
const isMobile = useIsMobile()

// ---------------------------------------------------------------- 列表与筛选

const loading = ref(false)
const items = ref<Authorizer[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
/** 勾选行（row-key 为 appid），供「同步选中」使用；跨页保留。 */
const checkedRowKeys = ref<DataTableRowKey[]>([])

/** 工具条筛选条件（点「查询」或回车后用于请求）。 */
const filters = reactive({
  keyword: '',
  status: 'all' as 'all' | AuthorizationStatus,
  groupName: '',
  tag: '',
  /** 仅看缺开发权限集（权限集 18）：为 true 时显式传 hasDevPermission=false。 */
  onlyMissingDevPermission: false,
})

const statusOptions: SelectOption[] = [
  { label: '全部授权状态', value: 'all' },
  { label: '已授权', value: 'authorized' },
  { label: '已取消授权', value: 'unauthorized' },
]

/** 组装列表查询参数：空值不传；「仅看缺权限集」显式传 hasDevPermission=false，默认不传该字段。 */
function buildListQuery(): {
  page: number
  pageSize: number
  keyword?: string
  status?: AuthorizationStatus
  groupName?: string
  tag?: string
  hasDevPermission?: boolean
} {
  const query: {
    page: number
    pageSize: number
    keyword?: string
    status?: AuthorizationStatus
    groupName?: string
    tag?: string
    hasDevPermission?: boolean
  } = { page: page.value, pageSize: pageSize.value }

  const keyword = filters.keyword.trim()
  if (keyword !== '') {
    query.keyword = keyword
  }
  if (filters.status !== 'all') {
    query.status = filters.status
  }
  const groupName = filters.groupName.trim()
  if (groupName !== '') {
    query.groupName = groupName
  }
  const tag = filters.tag.trim()
  if (tag !== '') {
    query.tag = tag
  }
  if (filters.onlyMissingDevPermission) {
    query.hasDevPermission = false
  }
  return query
}

/** 加载列表；勾选状态跨页保留，因此不按当前页裁剪 checkedRowKeys。 */
async function loadList(): Promise<void> {
  loading.value = true
  try {
    const { data, error } = await client.GET('/authorizers', { params: { query: buildListQuery() } })
    if (error || !data) {
      message.error(errorMessage(error))
      return
    }
    items.value = data.items
    total.value = data.total
  } finally {
    loading.value = false
  }
}

/** 查询：回到第一页重新加载。 */
function handleSearch(): void {
  page.value = 1
  void loadList()
}

/** 重置：清空全部筛选条件并回到第一页。 */
function handleReset(): void {
  filters.keyword = ''
  filters.status = 'all'
  filters.groupName = ''
  filters.tag = ''
  filters.onlyMissingDevPermission = false
  page.value = 1
  void loadList()
}

/** 复制文本到剪贴板。 */
async function copyText(text: string, label: string): Promise<void> {
  if (text.trim() === '') {
    message.warning(`${label}为空，没有可复制的内容`)
    return
  }
  try {
    await navigator.clipboard.writeText(text)
    message.success(`${label}已复制`)
  } catch {
    message.error('浏览器拒绝访问剪贴板，请手动选择复制')
  }
}

// ---------------------------------------------------------------- 同步操作

const syncing = ref(false)
const tokenLoading = ref(false)

/** 同步选中：按勾选的 appid 精确同步。 */
async function syncSelected(): Promise<void> {
  if (checkedRowKeys.value.length === 0) {
    message.warning('请先勾选需要同步的小程序')
    return
  }
  const appids = checkedRowKeys.value.map((key) => String(key))
  syncing.value = true
  try {
    const { data, error } = await client.POST('/authorizers/sync', { body: { appids } })
    if (error || !data) {
      message.error(errorMessage(error))
      return
    }
    message.success(syncSummaryText(data))
    await loadList()
  } finally {
    syncing.value = false
  }
}

/** 同步全部：空 body → 后端按「全部已授权且启用的小程序」全量同步。 */
async function syncAll(): Promise<void> {
  syncing.value = true
  try {
    const { data, error } = await client.POST('/authorizers/sync', { body: {} })
    if (error || !data) {
      message.error(errorMessage(error))
      return
    }
    message.success(syncSummaryText(data))
    await loadList()
  } finally {
    syncing.value = false
  }
}

/** 重新拉取令牌：从 api_get_authorizer_list 恢复全部 authorizer_refresh_token。 */
async function resyncTokens(): Promise<void> {
  tokenLoading.value = true
  try {
    const { data, error } = await client.POST('/authorizers/resync-tokens')
    if (error || !data) {
      message.error(errorMessage(error))
      return
    }
    message.success(`重新拉取令牌完成：更新 ${data.succeeded} 条，失败 ${data.failed} 条，共 ${data.total} 条`)
    await loadList()
  } finally {
    tokenLoading.value = false
  }
}

/** 同步结果文案。 */
function syncSummaryText(summary: SyncSummary): string {
  return `同步完成：成功 ${summary.succeeded} 个，失败 ${summary.failed} 个，共 ${summary.total} 个`
}

// ---------------------------------------------------------------- 列表渲染

/** 头像加载失败的小程序集合（失败后回退为首字头像）。 */
const avatarFailed = ref<Record<string, boolean>>({})

/** 取昵称首字，无昵称时用 appid 首字符。 */
function firstChar(nickName: string | null | undefined, appid: string): string {
  const name = nickName?.trim()
  if (name) {
    return name.slice(0, 1)
  }
  return appid.slice(0, 1).toUpperCase()
}

/** 头像列：优先展示微信头像，加载失败显示首字。 */
function renderAvatar(row: Authorizer): VNode {
  const failed = avatarFailed.value[row.appid] === true
  return h(
    NAvatar,
    {
      size: 34,
      round: true,
      src: failed ? undefined : row.headImg ?? undefined,
      onError: () => {
        avatarFailed.value = { ...avatarFailed.value, [row.appid]: true }
      },
    },
    { default: () => firstChar(row.nickName, row.appid) },
  )
}

/** 权限集 id → 中文说明（仅列关键项，其余按 id 展示）。 */
const PERMISSION_LABELS: Record<number, string> = {
  18: '小程序开发与数据分析',
}

/** 权限集列：id 列表 + 缺权限集 18 的警示标签。 */
function renderFuncInfoIds(row: Authorizer): VNode {
  const ids = row.funcInfoIds ?? []
  const children: VNode[] = []
  if (ids.length === 0) {
    children.push(h(NTag, { size: 'small', bordered: false }, { default: () => '无权限集' }))
  } else {
    for (const id of ids) {
      const label = PERMISSION_LABELS[id]
      children.push(
        h(
          NTag,
          { size: 'small', type: 'info', bordered: false },
          { default: () => (label ? `${id} · ${label}` : `权限集 ${id}`) },
        ),
      )
    }
  }
  if (!row.hasDevPermission) {
    children.push(
      h(
        NTag,
        { size: 'small', type: 'warning', bordered: false },
        { default: () => '缺权限集 18，无法代管代码' },
      ),
    )
  }
  return h('div', { class: 'tag-cell' }, children)
}

/** 标签列。 */
function renderTags(row: Authorizer): VNode {
  const tags = row.tags ?? []
  if (tags.length === 0) {
    return h('span', null, EMPTY_TEXT)
  }
  return h(
    'div',
    { class: 'tag-cell' },
    tags.map((tag) => h(NTag, { size: 'small', bordered: false, key: tag }, { default: () => tag })),
  )
}

/** row-key：appid（多选与详情定位都用它）。 */
function rowKey(row: Authorizer): string {
  return row.appid
}

const columns = computed<DataTableColumns<Authorizer>>(() => [
  { type: 'selection', width: 44 },
  { title: '头像', key: 'headImg', width: 72, render: (row) => renderAvatar(row) },
  { title: '昵称', key: 'nickName', width: 150, render: (row) => row.nickName ?? EMPTY_TEXT },
  {
    title: 'AppID',
    key: 'appid',
    width: 240,
    render: (row) =>
      h('div', { class: 'appid-cell' }, [
        h('span', { class: 'appid-text', title: row.appid }, truncateMiddle(row.appid, 10, 6)),
        h(
          NButton,
          { size: 'small', secondary: true, onClick: () => void copyText(row.appid, 'AppID') },
          { default: () => '复制' },
        ),
      ]),
  },
  {
    title: '授权状态',
    key: 'authorizationStatus',
    width: 110,
    render: (row) => h(StatusTag, { kind: 'authorizer', status: row.authorizationStatus }),
  },
  { title: '权限集', key: 'funcInfoIds', width: 240, render: (row) => renderFuncInfoIds(row) },
  { title: '分组', key: 'groupName', width: 110, render: (row) => row.groupName || EMPTY_TEXT },
  { title: '标签', key: 'tags', width: 150, render: (row) => renderTags(row) },
  {
    title: '最近同步',
    key: 'lastSyncAt',
    width: 120,
    render: (row) => formatRelativeTime(row.lastSyncAt),
  },
  {
    title: '操作',
    key: 'actions',
    width: 150,
    fixed: 'right',
    render: (row) =>
      h('div', { class: 'row-actions' }, [
        h(
          NButton,
          { size: 'small', onClick: () => void openDetail(row.appid) },
          { default: () => '详情' },
        ),
        h(NButton, { size: 'small', onClick: () => void openEdit(row) }, { default: () => '编辑' }),
      ]),
  },
])

// ---------------------------------------------------------------- 详情抽屉

const drawerOpen = ref(false)
const detailLoading = ref(false)
const detail = ref<AuthorizerDetail | null>(null)
const detailAppid = ref('')

/** 抽屉内异步动作的加载状态。 */
const actionLoading = reactive<Record<'sync' | 'token' | 'qr' | 'version' | 'visit' | 'pages', boolean>>({
  sync: false,
  token: false,
  qr: false,
  version: false,
  visit: false,
  pages: false,
})

const trialQrUrl = ref('')
const versionInfo = ref<VersionInfo | null>(null)
const visitStatus = ref<VisitStatus | null>(null)
const pageList = ref<string[]>([])

const drawerTitle = computed<string>(() => detail.value?.nickName || detailAppid.value || '小程序详情')

/** 拉取单个小程序详情。 */
async function fetchDetail(appid: string): Promise<AuthorizerDetail | null> {
  const { data, error } = await client.GET('/authorizers/{appid}', { params: { path: { appid } } })
  if (error || !data) {
    message.error(errorMessage(error))
    return null
  }
  return data
}

/** 打开详情抽屉：先清空上次的结果类面板，再拉取详情。 */
async function openDetail(appid: string): Promise<void> {
  detailAppid.value = appid
  drawerOpen.value = true
  detailLoading.value = true
  detail.value = null
  trialQrUrl.value = ''
  versionInfo.value = null
  visitStatus.value = null
  pageList.value = []
  try {
    const data = await fetchDetail(appid)
    if (!data) {
      drawerOpen.value = false
      return
    }
    detail.value = data
  } finally {
    detailLoading.value = false
  }
}

/** 抽屉开关：关闭时同步清理路由上的 appid，避免刷新后反复打开。 */
function handleDrawerShow(show: boolean): void {
  drawerOpen.value = show
  if (!show && (route.name === 'authorizer-detail' || route.query.appid !== undefined)) {
    void router.replace({ name: 'authorizers' })
  }
}

/** 从路由读取 appid（/authorizers/:appid 或 /authorizers?appid=xxx）自动打开详情。 */
function syncFromRoute(): void {
  const paramRaw = route.params.appid
  const paramAppid = Array.isArray(paramRaw) ? paramRaw[0] : paramRaw
  const queryRaw = route.query.appid
  const queryAppid = Array.isArray(queryRaw) ? queryRaw[0] : queryRaw
  const target =
    typeof paramAppid === 'string' && paramAppid !== ''
      ? paramAppid
      : typeof queryAppid === 'string' && queryAppid !== ''
        ? queryAppid
        : ''
  if (target !== '') {
    void openDetail(target)
  }
}

/** 同步信息（单小程序）。 */
async function syncDetail(): Promise<void> {
  if (detailAppid.value === '') {
    return
  }
  actionLoading.sync = true
  try {
    const { data, error } = await client.POST('/authorizers/{appid}/sync', {
      params: { path: { appid: detailAppid.value } },
    })
    if (error || !data) {
      message.error(errorMessage(error))
      return
    }
    detail.value = data
    message.success('已同步该小程序的最新信息')
    await loadList()
  } finally {
    actionLoading.sync = false
  }
}

/** 刷新令牌（全量重新拉取，成功后提示更新数量）。 */
async function refreshToken(): Promise<void> {
  actionLoading.token = true
  try {
    const { data, error } = await client.POST('/authorizers/resync-tokens')
    if (error || !data) {
      message.error(errorMessage(error))
      return
    }
    message.success(`重新拉取令牌完成：更新 ${data.succeeded} 条，失败 ${data.failed} 条`)
  } finally {
    actionLoading.token = false
  }
}

/** 体验版二维码（base64 → data URL）。 */
async function loadTrialQrcode(): Promise<void> {
  if (detailAppid.value === '') {
    return
  }
  actionLoading.qr = true
  try {
    const { data, error } = await client.GET('/releases/{appid}/trial-qrcode', {
      params: { path: { appid: detailAppid.value } },
    })
    if (error || !data) {
      message.error(errorMessage(error))
      return
    }
    trialQrUrl.value = `data:${data.contentType};base64,${data.base64}`
  } finally {
    actionLoading.qr = false
  }
}

/** 版本信息（体验版 / 线上版）。 */
async function loadVersionInfo(): Promise<void> {
  if (detailAppid.value === '') {
    return
  }
  actionLoading.version = true
  try {
    const { data, error } = await client.GET('/releases/{appid}/version', {
      params: { path: { appid: detailAppid.value } },
    })
    if (error || !data) {
      message.error(errorMessage(error))
      return
    }
    versionInfo.value = data
  } finally {
    actionLoading.version = false
  }
}

/** 服务状态（0 已暂停 / 1 未暂停）。 */
async function loadVisitStatus(): Promise<void> {
  if (detailAppid.value === '') {
    return
  }
  actionLoading.visit = true
  try {
    const { data, error } = await client.GET('/apps/{appid}/visit-status', {
      params: { path: { appid: detailAppid.value } },
    })
    if (error || !data) {
      message.error(errorMessage(error))
      return
    }
    visitStatus.value = data
  } finally {
    actionLoading.visit = false
  }
}

/** 暂停 / 恢复服务。 */
async function setVisitPaused(paused: boolean): Promise<void> {
  if (detailAppid.value === '') {
    return
  }
  actionLoading.visit = true
  try {
    const { data, error } = await client.PUT('/apps/{appid}/visit-status', {
      params: { path: { appid: detailAppid.value } },
      body: { paused },
    })
    if (error || !data) {
      message.error(errorMessage(error))
      return
    }
    visitStatus.value = data
    message.success(paused ? '已暂停该小程序服务' : '已恢复该小程序服务')
  } finally {
    actionLoading.visit = false
  }
}

/** 页面列表（代码包内页面路径）。 */
async function loadPages(): Promise<void> {
  if (detailAppid.value === '') {
    return
  }
  actionLoading.pages = true
  try {
    const { data, error } = await client.GET('/apps/{appid}/pages', {
      params: { path: { appid: detailAppid.value } },
    })
    if (error || !data) {
      message.error(errorMessage(error))
      return
    }
    pageList.value = data
  } finally {
    actionLoading.pages = false
  }
}

/** 域名分组定义（按微信的域名类型分组展示）。 */
const DOMAIN_GROUPS: { key: keyof DomainSnapshot; label: string }[] = [
  { key: 'requestDomain', label: 'request 合法域名' },
  { key: 'wsRequestDomain', label: 'socket 合法域名' },
  { key: 'uploadDomain', label: 'uploadFile 合法域名' },
  { key: 'downloadDomain', label: 'downloadFile 合法域名' },
  { key: 'udpDomain', label: 'UDP 合法域名' },
  { key: 'tcpDomain', label: 'TCP 合法域名' },
  { key: 'businessDomain', label: '业务域名' },
]

/** 把域名快照展开成分组行；快照缺失时返回空数组（表示未探测）。 */
function domainGroups(snapshot: DomainSnapshot | null | undefined): { label: string; values: string[] }[] {
  if (!snapshot) {
    return []
  }
  return DOMAIN_GROUPS.map((group) => ({ label: group.label, values: snapshot[group.key] ?? [] }))
}

/** 账号状态文案（微信账号状态码）。 */
function accountStatusText(status: number | null | undefined): string {
  switch (status) {
    case 1:
      return '正常'
    case 14:
      return '已注销'
    case 16:
      return '已封禁'
    case 18:
      return '已告警'
    case 19:
      return '已冻结'
    default:
      return status === null || status === undefined ? EMPTY_TEXT : String(status)
  }
}

/** 代码来源文案。 */
function codeSourceText(source: CodeSource | undefined): string {
  switch (source) {
    case 'template':
      return '模板库下发'
    case 'direct_commit':
      return 'CI 直传'
    default:
      return EMPTY_TEXT
  }
}

/** JSON 对象美化展示。 */
function formatJson(value: Record<string, unknown> | null | undefined): string {
  if (!value || Object.keys(value).length === 0) {
    return EMPTY_TEXT
  }
  return JSON.stringify(value, null, 2)
}

/** 类目文案：一级 / 二级。 */
function categoryText(pair: components['schemas']['CategoryPair']): string {
  return `${pair.first} / ${pair.second}`
}

// ---------------------------------------------------------------- 编辑弹窗

const editOpen = ref(false)
const editSaving = ref(false)
const editLoading = ref(false)
const editAppid = ref('')
const editNickName = ref('')

/** 编辑表单（extVars / auditOverride 以 JSON 文本编辑，提交前校验）。 */
const editForm = reactive({
  remark: '',
  groupName: '',
  tags: [] as string[],
  enabled: true,
  extVarsText: '',
  auditProfileId: null as number | null,
  codeSource: 'template' as CodeSource,
  auditOverrideText: '',
})

const EXTVARS_PLACEHOLDER = 'JSON 对象，例如 {"theme":"blue"}'
const AUDIT_OVERRIDE_PLACEHOLDER = '留空表示不覆盖，例如 {"categoryList":[]}'

const codeSourceOptions: SelectOption[] = [
  { label: '模板库下发', value: 'template' },
  { label: 'CI 直传', value: 'direct_commit' },
]

const editModalStyle = { width: 'min(640px, 94vw)', maxWidth: '94vw' }

/** 打开编辑弹窗：先用列表行数据填充，再拉一次详情补齐 auditOverride。 */
async function openEdit(row: Authorizer): Promise<void> {
  editAppid.value = row.appid
  editNickName.value = row.nickName ?? row.appid
  editForm.remark = row.remark ?? ''
  editForm.groupName = row.groupName ?? ''
  editForm.tags = [...(row.tags ?? [])]
  editForm.enabled = row.enabled
  editForm.extVarsText = row.extVars && Object.keys(row.extVars).length > 0 ? JSON.stringify(row.extVars, null, 2) : ''
  editForm.auditProfileId = row.auditProfileId ?? null
  editForm.codeSource = row.codeSource ?? 'template'
  editForm.auditOverrideText = ''
  editOpen.value = true

  // 详情里才有 auditOverride；列表页拿不到时补一次详情查询。
  if (detail.value?.appid !== row.appid) {
    editLoading.value = true
    try {
      const data = await fetchDetail(row.appid)
      if (data) {
        editForm.auditOverrideText =
          data.auditOverride && Object.keys(data.auditOverride).length > 0
            ? JSON.stringify(data.auditOverride, null, 2)
            : ''
      }
    } finally {
      editLoading.value = false
    }
  } else {
    const current = detail.value
    editForm.auditOverrideText =
      current.auditOverride && Object.keys(current.auditOverride).length > 0
        ? JSON.stringify(current.auditOverride, null, 2)
        : ''
  }
}

/** 从详情抽屉直接进入编辑。 */
function openEditFromDetail(): void {
  const current = detail.value
  if (!current) {
    return
  }
  void openEdit(current)
}

/** 解析 JSON 文本域：空文本 → null；非法 JSON / 非对象 → 中文错误。 */
function parseJsonField(
  text: string,
  label: string,
): { data?: Record<string, unknown> | null; error?: string } {
  const trimmed = text.trim()
  if (trimmed === '') {
    return { data: null }
  }
  let parsed: unknown
  try {
    parsed = JSON.parse(trimmed)
  } catch {
    return { error: `${label}不是合法的 JSON，请检查括号、引号与逗号` }
  }
  if (parsed === null) {
    return { data: null }
  }
  if (typeof parsed !== 'object' || Array.isArray(parsed)) {
    return { error: `${label}必须是 JSON 对象，例如 {"key":"value"}` }
  }
  return { data: parsed as Record<string, unknown> }
}

/** 保存编辑（PATCH /authorizers/{appid}）。 */
async function submitEdit(): Promise<void> {
  if (editAppid.value === '') {
    return
  }
  const extResult = parseJsonField(editForm.extVarsText, 'ext 变量')
  if (extResult.error) {
    message.error(extResult.error)
    return
  }
  const extVars: Record<string, string> = {}
  for (const [key, value] of Object.entries(extResult.data ?? {})) {
    if (typeof value !== 'string') {
      message.error(`ext 变量的值必须是字符串：「${key}」不是字符串`)
      return
    }
    extVars[key] = value
  }
  const overrideResult = parseJsonField(editForm.auditOverrideText, '提审覆盖配置')
  if (overrideResult.error) {
    message.error(overrideResult.error)
    return
  }

  const remark = editForm.remark.trim()
  const groupName = editForm.groupName.trim()
  const body: AuthorizerUpdateRequest = {
    remark: remark === '' ? null : remark,
    groupName: groupName === '' ? null : groupName,
    tags: editForm.tags.length > 0 ? editForm.tags : null,
    enabled: editForm.enabled,
    extVars,
    auditProfileId: editForm.auditProfileId,
    codeSource: editForm.codeSource,
    auditOverride: overrideResult.data ?? null,
  }

  editSaving.value = true
  try {
    const { data, error } = await client.PATCH('/authorizers/{appid}', {
      params: { path: { appid: editAppid.value } },
      body,
    })
    if (error || !data) {
      message.error(errorMessage(error))
      return
    }
    if (detail.value?.appid === data.appid) {
      detail.value = data
    }
    editOpen.value = false
    message.success('已保存')
    await loadList()
  } finally {
    editSaving.value = false
  }
}

/** 删除（弹窗底部左侧按钮）：先二次确认，再 DELETE；409 直接展示后端说明。 */
function handleDelete(): void {
  const appid = editAppid.value
  if (appid === '') {
    return
  }
  dialog.warning({
    title: '删除小程序记录',
    content: `确定删除「${editNickName.value}」吗？只有「已取消授权」的小程序可以删除，删除后记录不可恢复。`,
    positiveText: '确认删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      const { error } = await client.DELETE('/authorizers/{appid}', {
        params: { path: { appid } },
      })
      if (error) {
        message.error(errorMessage(error))
        return
      }
      editOpen.value = false
      drawerOpen.value = false
      message.success('已删除')
      await loadList()
    },
  })
}

// ---------------------------------------------------------------- 生成授权链接

const authOpen = ref(false)
const authGenerating = ref(false)
const authResult = ref<AuthorizationUrlResponse | null>(null)
/** authType 用 number 存储以便下拉绑定，请求时收窄为契约的 AuthType。 */
const authForm = reactive({
  authType: 2,
  bizAppid: '',
  redirectUri: '',
  categoryIdList: '',
})

const authTypeOptions: SelectOption[] = [
  { label: '1 · 仅公众号', value: 1 },
  { label: '2 · 仅小程序（默认）', value: 2 },
  { label: '3 · 公众号 + 小程序', value: 3 },
  { label: '4 · 小程序推客', value: 4 },
  { label: '5 · 视频号', value: 5 },
  { label: '6 · 全部', value: 6 },
  { label: '8 · 带货助手', value: 8 },
]

const authModalStyle = { width: 'min(640px, 94vw)', maxWidth: '94vw' }

/** 打开生成授权链接弹窗：默认回填平台配置的授权回调地址。 */
async function openAuthDialog(): Promise<void> {
  authOpen.value = true
  authResult.value = null
  if (authForm.redirectUri === '') {
    const { data, error } = await client.GET('/platform/status')
    if (error || !data) {
      message.error(errorMessage(error))
      return
    }
    authForm.redirectUri = data.authRedirectUri ?? ''
  }
}

/** 生成授权链接：GET /authorizers/authorization-url。 */
async function generateAuthUrl(): Promise<void> {
  authGenerating.value = true
  try {
    const query: { authType: AuthType; bizAppid?: string; redirectUri?: string; categoryIdList?: string } = {
      authType: authForm.authType as AuthType,
    }
    const bizAppid = authForm.bizAppid.trim()
    if (bizAppid !== '') {
      query.bizAppid = bizAppid
    }
    const redirectUri = authForm.redirectUri.trim()
    if (redirectUri !== '') {
      query.redirectUri = redirectUri
    }
    const categoryIdList = authForm.categoryIdList.trim()
    if (categoryIdList !== '') {
      query.categoryIdList = categoryIdList
    }
    const { data, error } = await client.GET('/authorizers/authorization-url', {
      params: { query },
    })
    if (error || !data) {
      message.error(errorMessage(error))
      return
    }
    authResult.value = data
  } finally {
    authGenerating.value = false
  }
}

onMounted(() => {
  syncFromRoute()
  void loadList()
})

// 「仅看缺开发权限集」开关是布尔筛选，切换后立即回到第一页重新查询。
watch(
  () => filters.onlyMissingDevPermission,
  () => {
    page.value = 1
    void loadList()
  },
)

// 路由参数变化（例如从体检页跳转到 /authorizers?appid=xxx）时打开对应详情。
watch(
  () => route.fullPath,
  () => {
    if (route.name === 'authorizer-detail' || route.name === 'authorizers') {
      syncFromRoute()
    }
  },
)
</script>

<template>
  <div class="page">
    <page-header title="小程序管理" description="授权方清单、授权链接生成、信息同步与详情维护">
      <template #extra>
        <n-button type="primary" @click="openAuthDialog">
          <template #icon>
            <n-icon><add-outline /></n-icon>
          </template>
          生成授权链接
        </n-button>
        <n-button :loading="syncing" :disabled="checkedRowKeys.length === 0" @click="syncSelected">
          <template #icon>
            <n-icon><sync-outline /></n-icon>
          </template>
          同步选中{{ checkedRowKeys.length > 0 ? `（${checkedRowKeys.length}）` : '' }}
        </n-button>
        <n-button :loading="tokenLoading" @click="resyncTokens">
          <template #icon>
            <n-icon><refresh-outline /></n-icon>
          </template>
          重新拉取令牌
        </n-button>
        <n-button :loading="syncing" @click="syncAll">
          <template #icon>
            <n-icon><sync-outline /></n-icon>
          </template>
          同步全部
        </n-button>
      </template>
    </page-header>

    <n-card :bordered="true">
      <div class="page-toolbar">
        <n-input
          v-model:value="filters.keyword"
          class="toolbar-item"
          clearable
          placeholder="昵称 / AppID / 备注 / 主体名称"
          @keyup.enter="handleSearch"
        />
        <n-select v-model:value="filters.status" class="toolbar-item-narrow" :options="statusOptions" />
        <n-input
          v-model:value="filters.groupName"
          class="toolbar-item"
          clearable
          placeholder="分组"
          @keyup.enter="handleSearch"
        />
        <n-input
          v-model:value="filters.tag"
          class="toolbar-item"
          clearable
          placeholder="标签"
          @keyup.enter="handleSearch"
        />
        <div class="toolbar-switch">
          <span class="toolbar-switch-label">仅看缺开发权限集</span>
          <n-switch v-model:value="filters.onlyMissingDevPermission" />
        </div>
        <n-button type="primary" @click="handleSearch">查询</n-button>
        <n-button @click="handleReset">重置</n-button>
      </div>

      <n-data-table
        v-model:checked-row-keys="checkedRowKeys"
        :columns="columns"
        :data="items"
        :loading="loading"
        :row-key="rowKey"
        :bordered="false"
        :scroll-x="1400"
        size="small"
      />

      <div class="pager">
        <n-pagination
          v-model:page="page"
          v-model:page-size="pageSize"
          :item-count="total"
          :simple="isMobile"
          :page-sizes="[10, 20, 50, 100]"
          show-size-picker
          @update:page="loadList"
          @update:page-size="loadList"
        />
      </div>
    </n-card>

    <!-- 详情抽屉：全量资料 + 单小程序操作 -->
    <n-drawer
      :show="drawerOpen"
      :width="'min(720px, 94vw)'"
      placement="right"
      @update:show="handleDrawerShow"
    >
      <n-drawer-content :title="drawerTitle" closable :native-scrollbar="false">
        <div class="drawer-actions">
          <n-button size="small" :loading="actionLoading.sync" @click="syncDetail">同步信息</n-button>
          <n-button size="small" :loading="actionLoading.token" @click="refreshToken">刷新令牌</n-button>
          <n-button size="small" :loading="actionLoading.qr" @click="loadTrialQrcode">体验版二维码</n-button>
          <n-button size="small" :loading="actionLoading.version" @click="loadVersionInfo">版本信息</n-button>
          <n-button size="small" :loading="actionLoading.visit" @click="loadVisitStatus">服务状态</n-button>
          <n-button size="small" :loading="actionLoading.pages" @click="loadPages">页面列表</n-button>
          <n-button size="small" type="primary" secondary @click="openEditFromDetail">编辑</n-button>
        </div>

        <div v-if="detailLoading" class="detail-loading">
          <n-spin size="small" />
          <span>正在加载小程序详情…</span>
        </div>
        <n-empty v-else-if="!detail" description="没有可展示的小程序详情" />

        <template v-else>
          <div class="detail-section">
            <div class="detail-section-title">基础资料</div>
            <div class="detail-row">
              <span class="detail-label">小程序昵称</span>
              <span class="detail-value">{{ detail.nickName || EMPTY_TEXT }}</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">AppID</span>
              <span class="detail-value">
                <span class="mono">{{ detail.appid }}</span>
                <n-button size="small" secondary @click="copyText(detail.appid, 'AppID')">
                  <template #icon>
                    <n-icon><copy-outline /></n-icon>
                  </template>
                  复制
                </n-button>
              </span>
            </div>
            <div class="detail-row">
              <span class="detail-label">原始 ID</span>
              <span class="detail-value">{{ detail.userName || EMPTY_TEXT }}</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">主体名称</span>
              <span class="detail-value">{{ detail.principalName || EMPTY_TEXT }}</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">账号状态</span>
              <span class="detail-value">{{ accountStatusText(detail.accountStatus) }}</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">服务类型 / 认证类型</span>
              <span class="detail-value">
                {{ detail.serviceTypeId ?? EMPTY_TEXT }} / {{ detail.verifyTypeId ?? EMPTY_TEXT }}
                （注册类型 {{ detail.registerType ?? EMPTY_TEXT }}）
              </span>
            </div>
            <div class="detail-row">
              <span class="detail-label">别名</span>
              <span class="detail-value">{{ detail.alias || EMPTY_TEXT }}</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">简介</span>
              <span class="detail-value">{{ detail.signature || EMPTY_TEXT }}</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">授权状态</span>
              <span class="detail-value">
                <status-tag kind="authorizer" :status="detail.authorizationStatus" />
              </span>
            </div>
            <div class="detail-row">
              <span class="detail-label">备注</span>
              <span class="detail-value">{{ detail.remark || EMPTY_TEXT }}</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">分组 / 标签</span>
              <span class="detail-value">
                {{ detail.groupName || EMPTY_TEXT }}
                <n-tag v-for="tag in detail.tags ?? []" :key="tag" size="small" :bordered="false">
                  {{ tag }}
                </n-tag>
              </span>
            </div>
            <div class="detail-row">
              <span class="detail-label">启用状态</span>
              <span class="detail-value">
                <status-tag kind="boolean" :status="detail.enabled" :label="detail.enabled ? '已启用' : '已停用'" />
              </span>
            </div>
          </div>

          <div class="detail-section">
            <div class="detail-section-title">权限集与代码来源</div>
            <div class="detail-row">
              <span class="detail-label">权限集 ID</span>
              <span class="detail-value">
                <n-tag
                  v-for="id in detail.funcInfoIds ?? []"
                  :key="id"
                  size="small"
                  :bordered="false"
                  type="info"
                >
                  {{ id }}
                </n-tag>
                <span v-if="(detail.funcInfoIds ?? []).length === 0">{{ EMPTY_TEXT }}</span>
                <n-tag v-if="!detail.hasDevPermission" size="small" type="warning" :bordered="false">
                  缺权限集 18，无法代管代码
                </n-tag>
              </span>
            </div>
            <div class="detail-row">
              <span class="detail-label">权限集说明</span>
              <span class="detail-value">
                18 = 小程序开发与数据分析，是代码管理（上传 / 提审 / 发布）的前提，且与其他权限集互斥；
                缺 18 需让管理员重新授权并单独勾选。
              </span>
            </div>
            <div class="detail-row">
              <span class="detail-label">代码来源</span>
              <span class="detail-value">{{ codeSourceText(detail.codeSource) }}</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">提审配置 ID</span>
              <span class="detail-value">{{ detail.auditProfileId ?? EMPTY_TEXT }}</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">类目</span>
              <span class="detail-value">
                <n-tag
                  v-for="(pair, index) in detail.miniProgramCategories ?? []"
                  :key="index"
                  size="small"
                  :bordered="false"
                >
                  {{ categoryText(pair) }}
                </n-tag>
                <span v-if="(detail.miniProgramCategories ?? []).length === 0">{{ EMPTY_TEXT }}</span>
              </span>
            </div>
            <div class="detail-row">
              <span class="detail-label">ext 变量</span>
              <span class="detail-value">
                <pre class="json-block">{{ formatJson(detail.extVars) }}</pre>
              </span>
            </div>
            <div class="detail-row">
              <span class="detail-label">提审覆盖配置</span>
              <span class="detail-value">
                <pre class="json-block">{{ formatJson(detail.auditOverride) }}</pre>
              </span>
            </div>
          </div>

          <div class="detail-section">
            <div class="detail-section-title">域名快照</div>
            <div class="detail-subtitle">已生效域名</div>
            <div v-if="domainGroups(detail.effectiveDomains).length === 0" class="detail-row">
              <span class="detail-value">未探测</span>
            </div>
            <div
              v-for="group in domainGroups(detail.effectiveDomains)"
              :key="`eff-${group.label}`"
              class="detail-row"
            >
              <span class="detail-label">{{ group.label }}</span>
              <span class="detail-value">
                <n-tag v-for="domain in group.values" :key="domain" size="small" :bordered="false">
                  {{ domain }}
                </n-tag>
                <span v-if="group.values.length === 0">{{ EMPTY_TEXT }}</span>
              </span>
            </div>

            <div class="detail-subtitle">已登记域名</div>
            <div v-if="domainGroups(detail.registeredDomains).length === 0" class="detail-row">
              <span class="detail-value">未探测</span>
            </div>
            <div
              v-for="group in domainGroups(detail.registeredDomains)"
              :key="`reg-${group.label}`"
              class="detail-row"
            >
              <span class="detail-label">{{ group.label }}</span>
              <span class="detail-value">
                <n-tag v-for="domain in group.values" :key="domain" size="small" :bordered="false">
                  {{ domain }}
                </n-tag>
                <span v-if="group.values.length === 0">{{ EMPTY_TEXT }}</span>
              </span>
            </div>
          </div>

          <div class="detail-section">
            <div class="detail-section-title">时间与令牌</div>
            <div class="detail-row">
              <span class="detail-label">授权时间</span>
              <span class="detail-value">{{ formatDateTime(detail.authorizedAt) }}</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">最近同步</span>
              <span class="detail-value">
                {{ formatDateTime(detail.lastSyncAt) }}（{{ formatRelativeTime(detail.lastSyncAt) }}）
              </span>
            </div>
            <div class="detail-row">
              <span class="detail-label">令牌更新时间</span>
              <span class="detail-value">{{ formatDateTime(detail.refreshTokenUpdatedAt) }}</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">用户隐私保护指引</span>
              <span class="detail-value">
                <template v-if="detail.privacySettingConfigured === null || detail.privacySettingConfigured === undefined">
                  未探测
                </template>
                <template v-else>
                  <status-tag
                    kind="boolean"
                    :status="detail.privacySettingConfigured"
                    :label="detail.privacySettingConfigured ? '已配置' : '未配置'"
                  />
                </template>
                <span>提审前必须配置用户隐私保护指引，否则提审会被拒绝。</span>
              </span>
            </div>
            <div class="detail-row">
              <span class="detail-label">最近体检</span>
              <span class="detail-value">{{ formatDateTime(detail.lastPreflightAt) }}</span>
            </div>
          </div>

          <div v-if="trialQrUrl" class="detail-section">
            <div class="detail-section-title">体验版二维码</div>
            <n-image :src="trialQrUrl" width="200" />
          </div>

          <div v-if="versionInfo" class="detail-section">
            <div class="detail-section-title">版本信息</div>
            <div class="detail-row">
              <span class="detail-label">体验版</span>
              <span class="detail-value">
                {{ versionInfo.expVersion || EMPTY_TEXT }} ·
                {{ versionInfo.expDesc || EMPTY_TEXT }} ·
                {{ formatDateTime(versionInfo.expTime) }}
              </span>
            </div>
            <div class="detail-row">
              <span class="detail-label">线上版</span>
              <span class="detail-value">
                {{ versionInfo.releaseVersion || EMPTY_TEXT }} ·
                {{ versionInfo.releaseDesc || EMPTY_TEXT }} ·
                {{ formatDateTime(versionInfo.releaseTime) }}
              </span>
            </div>
          </div>

          <div v-if="visitStatus" class="detail-section">
            <div class="detail-section-title">服务状态</div>
            <div class="detail-row">
              <span class="detail-label">当前状态</span>
              <span class="detail-value">
                <status-tag
                  kind="boolean"
                  :status="!visitStatus.paused"
                  :label="visitStatus.paused ? '已暂停服务' : '服务正常'"
                />
              </span>
            </div>
            <div class="detail-actions">
              <n-button size="small" :disabled="visitStatus.paused" @click="setVisitPaused(true)">
                暂停服务
              </n-button>
              <n-button size="small" :disabled="!visitStatus.paused" @click="setVisitPaused(false)">
                恢复服务
              </n-button>
            </div>
          </div>

          <div v-if="pageList.length > 0" class="detail-section">
            <div class="detail-section-title">页面列表</div>
            <div class="detail-row">
              <span class="detail-value">
                <n-tag v-for="page in pageList" :key="page" size="small" :bordered="false">
                  {{ page }}
                </n-tag>
              </span>
            </div>
          </div>
        </template>
      </n-drawer-content>
    </n-drawer>

    <!-- 编辑弹窗：删除按钮在底部左侧 -->
    <n-modal
      v-model:show="editOpen"
      preset="card"
      title="编辑小程序"
      :style="editModalStyle"
      :mask-closable="false"
    >
      <n-form label-placement="top">
        <n-form-item label="编辑对象">
          <span class="form-static">{{ editNickName }}（{{ editAppid }}）</span>
        </n-form-item>
        <n-form-item label="备注">
          <n-input v-model:value="editForm.remark" type="textarea" :rows="3" placeholder="内部备注，便于检索与交接" />
        </n-form-item>
        <n-form-item label="分组">
          <n-input v-model:value="editForm.groupName" placeholder="例如 直营 / 加盟商" />
        </n-form-item>
        <n-form-item label="标签">
          <n-dynamic-tags v-model:value="editForm.tags" />
        </n-form-item>
        <n-form-item label="启用（停用后不参与批量任务）">
          <n-switch v-model:value="editForm.enabled" />
        </n-form-item>
        <n-form-item label="代码来源">
          <n-select v-model:value="editForm.codeSource" :options="codeSourceOptions" />
        </n-form-item>
        <n-form-item label="提审配置 ID">
          <n-input-number v-model:value="editForm.auditProfileId" clearable placeholder="留空使用平台默认配置" />
        </n-form-item>
        <n-form-item label="ext 变量（JSON 对象）">
          <n-input
            v-model:value="editForm.extVarsText"
            type="textarea"
            :rows="5"
            :placeholder="EXTVARS_PLACEHOLDER"
          />
        </n-form-item>
        <n-form-item label="提审覆盖配置（JSON 对象，可选）">
          <n-input
            v-model:value="editForm.auditOverrideText"
            type="textarea"
            :rows="5"
            :placeholder="AUDIT_OVERRIDE_PLACEHOLDER"
          />
        </n-form-item>
      </n-form>

      <template #footer>
        <div class="modal-footer">
          <div class="modal-footer-left">
            <n-button type="error" secondary @click="handleDelete">删除</n-button>
          </div>
          <div class="modal-footer-right">
            <n-button :disabled="editLoading || editSaving" @click="editOpen = false">取消</n-button>
            <n-button type="primary" :loading="editSaving" @click="submitEdit">保存</n-button>
          </div>
        </div>
      </template>
    </n-modal>

    <!-- 生成授权链接弹窗 -->
    <n-modal
      v-model:show="authOpen"
      preset="card"
      title="生成授权链接"
      :style="authModalStyle"
      :mask-closable="false"
    >
      <n-form label-placement="top">
        <n-form-item label="授权类型">
          <n-select v-model:value="authForm.authType" :options="authTypeOptions" />
        </n-form-item>
        <n-form-item label="指定小程序 AppID（可选）">
          <n-input
            v-model:value="authForm.bizAppid"
            placeholder="指定后只有该小程序管理员可授权，优先级高于授权类型"
          />
        </n-form-item>
        <n-form-item label="授权回调地址">
          <n-input v-model:value="authForm.redirectUri" placeholder="需与开放平台「授权发起页域名」一致" />
        </n-form-item>
        <n-form-item label="权限集 ID（可选，逗号分隔）">
          <n-input v-model:value="authForm.categoryIdList" placeholder="例如 18,30；不填使用已发布权限集" />
        </n-form-item>
      </n-form>

      <div v-if="authResult" class="auth-result">
        <div class="detail-row">
          <span class="detail-label">PC 链接</span>
          <span class="detail-value">
            <span class="mono break-all">{{ authResult.pcUrl }}</span>
            <n-button size="small" secondary @click="copyText(authResult.pcUrl, 'PC 链接')">
              <template #icon>
                <n-icon><copy-outline /></n-icon>
              </template>
              复制
            </n-button>
          </span>
        </div>
        <div class="detail-row">
          <span class="detail-label">移动端链接</span>
          <span class="detail-value">
            <span class="mono break-all">{{ authResult.mobileUrl }}</span>
            <n-button size="small" secondary @click="copyText(authResult.mobileUrl, '移动端链接')">
              <template #icon>
                <n-icon><copy-outline /></n-icon>
              </template>
              复制
            </n-button>
          </span>
        </div>
        <div class="detail-row">
          <span class="detail-label">预授权码有效期</span>
          <span class="detail-value">{{ authResult.expiresInSeconds }} 秒</span>
        </div>
        <div class="auth-qr">
          <n-qr-code :value="authResult.qrCodeContent" :size="180" />
          <span class="auth-qr-note">请把 PC 链接或二维码发给小程序管理员；授权完成后微信会跳回本平台并自动登记该小程序。</span>
        </div>
      </div>

      <template #footer>
        <div class="modal-footer">
          <div class="modal-footer-left" />
          <div class="modal-footer-right">
            <n-button @click="authOpen = false">关闭</n-button>
            <n-button type="primary" :loading="authGenerating" @click="generateAuthUrl">生成</n-button>
          </div>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.toolbar-item {
  width: 220px;
}

.toolbar-item-narrow {
  width: 160px;
}

.toolbar-switch {
  display: flex;
  align-items: center;
  gap: 8px;
}

.toolbar-switch-label {
  font-size: 14px;
  color: #1f2937;
}

.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 14px;
}

.appid-cell {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.appid-text {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 14px;
  color: #1f2937;
  word-break: break-all;
}

.row-actions {
  display: flex;
  gap: 8px;
}

.tag-cell {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.drawer-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding-bottom: 12px;
  margin-bottom: 4px;
  border-bottom: 1px solid #eef2f8;
}

.detail-section {
  padding: 12px 0;
  border-bottom: 1px solid #eef2f8;
}

.detail-section:last-child {
  border-bottom: none;
}

.detail-section-title {
  font-size: 15px;
  font-weight: 600;
  color: #17233d;
  margin-bottom: 8px;
}

.detail-subtitle {
  font-size: 14px;
  font-weight: 600;
  color: #3d4a5c;
  margin: 8px 0 4px;
}

.detail-row {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 6px 0;
  flex-wrap: wrap;
}

.detail-label {
  width: 170px;
  flex-shrink: 0;
  font-size: 14px;
  font-weight: 600;
  color: #3d4a5c;
}

.detail-value {
  flex: 1;
  min-width: 200px;
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  font-size: 14px;
  line-height: 1.6;
  color: #1f2937;
  word-break: break-all;
}

.detail-loading {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 0;
  font-size: 14px;
  color: #1f2937;
}

.detail-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 8px;
}

.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}

.break-all {
  word-break: break-all;
}

.json-block {
  margin: 0;
  padding: 8px 10px;
  width: 100%;
  max-height: 220px;
  overflow: auto;
  background: #f7f9fc;
  border: 1px solid #e3ebf5;
  border-radius: 6px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 13px;
  color: #1f2937;
}

.auth-result {
  margin-top: 8px;
}

.auth-qr {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
  padding-top: 8px;
}

.auth-qr-note {
  flex: 1;
  min-width: 200px;
  font-size: 14px;
  line-height: 1.6;
  color: #1f2937;
}

.form-static {
  font-size: 14px;
  color: #1f2937;
  word-break: break-all;
}

.modal-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  flex-wrap: wrap;
}

.modal-footer-left,
.modal-footer-right {
  display: flex;
  gap: 8px;
}

@media (max-width: 820px) {
  .toolbar-item,
  .toolbar-item-narrow {
    width: 100%;
  }

  .detail-label {
    width: 100%;
  }

  .modal-footer {
    flex-direction: column-reverse;
    align-items: stretch;
  }

  .modal-footer-left :deep(.n-button),
  .modal-footer-right :deep(.n-button) {
    width: 100%;
  }

  .modal-footer-right :deep(.n-button) {
    flex: 1;
  }
}
</style>
