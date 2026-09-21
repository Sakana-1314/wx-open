<script setup lang="ts">
// 前置体检页：选择体检目的 + 目标小程序（全部 / 手动勾选）+ 可选域名要求，
// 调 POST /preflight 后逐个小程序展示 ready 与各项检查（label + 状态 + message + hint）。
import { computed, h, onMounted, ref, type VNode } from 'vue'
import { useRouter } from 'vue-router'
import {
  NButton,
  NCard,
  NDataTable,
  NForm,
  NFormItem,
  NInput,
  NList,
  NListItem,
  NRadioButton,
  NRadioGroup,
  NSelect,
  NTag,
  useMessage,
  type DataTableColumns,
  type SelectOption,
} from 'naive-ui'

import { client, errorMessage } from '@/api/client'
import PageHeader from '@/components/PageHeader.vue'
import StatusTag from '@/components/StatusTag.vue'
import type { components } from '@/api/schema'
import { formatQuota, type TagType } from '@/utils/format'

type PreflightItem = components['schemas']['PreflightItem']
type PreflightRequest = components['schemas']['PreflightRequest']
type PreflightResponse = components['schemas']['PreflightResponse']
type PreflightCheck = components['schemas']['PreflightCheck']
type DomainRequirements = components['schemas']['DomainRequirements']
/** 单条检查项的状态枚举。 */
type CheckStatus = PreflightCheck['status']
/** 体检目的枚举（契约里是字符串枚举）。 */
type Purpose = PreflightRequest['purpose']

const router = useRouter()
const message = useMessage()

const loading = ref(false)
const result = ref<PreflightResponse | null>(null)

/** 体检目的：默认提审（检查项最全）。 */
const purpose = ref<Purpose>('submit_audit')
/** 选择方式：全部已授权小程序 / 手动勾选 AppID。 */
const selectionMode = ref<'all' | 'manual'>('all')
const selectedAppids = ref<string[]>([])
/** 可选的要求域名（逗号分隔输入）。 */
const requestDomainText = ref('')
const businessDomainText = ref('')

const optionsLoading = ref(false)
const appidOptions = ref<SelectOption[]>([])

const purposeOptions: SelectOption[] = [
  { label: 'commit · 上传代码', value: 'commit' },
  { label: 'submit_audit · 提审', value: 'submit_audit' },
  { label: 'release · 发布', value: 'release' },
]

/** 体检结果表格列：展开行展示各项检查。 */
const columns = computed<DataTableColumns<PreflightItem>>(() => [
  { type: 'expand', width: 44, renderExpand: (row) => renderChecks(row) },
  {
    title: '小程序',
    key: 'appid',
    minWidth: 260,
    render: (row) =>
      h('div', { class: 'app-cell' }, [
        h('div', { class: 'app-name' }, row.nickName || '未获取昵称'),
        h('div', { class: 'app-id' }, row.appid),
      ]),
  },
  {
    title: '结论',
    key: 'ready',
    width: 140,
    render: (row) =>
      h(StatusTag, {
        kind: 'boolean',
        status: row.ready,
        label: row.ready ? '可执行' : '有阻塞',
      }),
  },
  {
    title: '阻塞项',
    key: 'failed',
    width: 100,
    render: (row) => String(failedCount(row)),
  },
])

/** 单个小程序的失败检查项数量。 */
function failedCount(item: PreflightItem): number {
  return item.checks.filter((check) => check.status === 'fail').length
}

/** 检查项状态 → 标签颜色（pass 绿 / warn 橙 / fail 红 / unknown 灰）。 */
function checkStatusType(status: CheckStatus): TagType {
  switch (status) {
    case 'pass':
      return 'success'
    case 'warn':
      return 'warning'
    case 'fail':
      return 'error'
    default:
      return 'default'
  }
}

/** 检查项状态 → 中文文案。 */
function checkStatusText(status: CheckStatus): string {
  switch (status) {
    case 'pass':
      return '通过'
    case 'warn':
      return '警告'
    case 'fail':
      return '失败'
    default:
      return '未探测'
  }
}

/** 展开行：逐条展示检查项（label + 状态标签 + message + hint），fail 项给「去处理」。 */
function renderChecks(item: PreflightItem): VNode {
  const rows = item.checks.map((check) =>
    h(
      NListItem,
      { key: check.key },
      {
        default: () =>
          h('div', { class: 'check-item' }, [
            h('div', { class: 'check-head' }, [
              h('span', { class: 'check-label' }, check.label),
              h(
                NTag,
                {
                  size: 'small',
                  round: true,
                  bordered: false,
                  type: checkStatusType(check.status),
                },
                { default: () => checkStatusText(check.status) },
              ),
            ]),
            h('div', { class: 'check-message' }, check.message),
            check.hint ? h('div', { class: 'check-hint' }, `处理建议：${check.hint}`) : null,
            check.status === 'fail'
              ? h(
                  NButton,
                  {
                    size: 'small',
                    type: 'primary',
                    secondary: true,
                    onClick: () => goAuthorizer(item.appid),
                  },
                  { default: () => '去处理' },
                )
              : null,
          ]),
      },
    ),
  )
  return h(
    'div',
    { class: 'check-wrap' },
    [
      h(
        NList,
        { bordered: true, showDivider: true },
        { default: () => rows },
      ),
    ],
  )
}

/** 「去处理」：跳转到小程序管理页并直接打开该小程序详情。 */
function goAuthorizer(appid: string): void {
  void router.push({ path: '/authorizers', query: { appid } })
}

/** 逗号（中英文）或空白分隔的域名列表 → 数组；空串返回空数组。 */
function splitDomains(text: string): string[] {
  return text
    .split(/[,，\s]+/)
    .map((item) => item.trim())
    .filter((item) => item !== '')
}

/** 加载小程序下拉选项（全部已授权小程序，最多 200 条）。 */
async function loadAppidOptions(): Promise<void> {
  optionsLoading.value = true
  try {
    const { data, error } = await client.GET('/authorizers', {
      params: { query: { pageSize: 200 } },
    })
    if (error || !data) {
      message.error(errorMessage(error))
      return
    }
    appidOptions.value = data.items.map((item) => ({
      label: `${item.nickName || item.appid}（${item.appid}）`,
      value: item.appid,
    }))
  } finally {
    optionsLoading.value = false
  }
}

/** 开始体检：组装 selection 与可选域名要求后调用 POST /preflight。 */
async function runPreflight(): Promise<void> {
  const appids = selectedAppids.value
  if (selectionMode.value === 'manual' && appids.length === 0) {
    message.warning('请至少选择一个小程序，或切换为「全部已授权小程序」')
    return
  }

  const body: PreflightRequest = {
    purpose: purpose.value,
    // 不传 appids 表示按筛选（全部已授权且启用的小程序）体检。
    selection: selectionMode.value === 'manual' ? { appids } : {},
  }

  const requiredDomains: DomainRequirements = {}
  const requestDomain = splitDomains(requestDomainText.value)
  if (requestDomain.length > 0) {
    requiredDomains.requestDomain = requestDomain
  }
  const businessDomain = splitDomains(businessDomainText.value)
  if (businessDomain.length > 0) {
    requiredDomains.businessDomain = businessDomain
  }
  if (requestDomain.length > 0 || businessDomain.length > 0) {
    body.requiredDomains = requiredDomains
  }

  loading.value = true
  try {
    const { data, error } = await client.POST('/preflight', { body })
    if (error || !data) {
      message.error(errorMessage(error))
      result.value = null
      return
    }
    result.value = data
    if (data.items.length === 0) {
      message.warning('没有匹配到需要体检的小程序')
    } else {
      message.success(`体检完成：可执行 ${data.readyCount} 个，有阻塞 ${data.blockedCount} 个`)
    }
  } finally {
    loading.value = false
  }
}

/** 重置表单与结果。 */
function resetForm(): void {
  purpose.value = 'submit_audit'
  selectionMode.value = 'all'
  selectedAppids.value = []
  requestDomainText.value = ''
  businessDomainText.value = ''
  result.value = null
}

/** 提审额度文案（服务商级，未探测时展示「未探测」）。 */
const auditQuotaText = computed<string>(() =>
  formatQuota(result.value?.auditQuota?.rest, result.value?.auditQuota?.limit),
)

/** 加急额度文案。 */
const speedupQuotaText = computed<string>(() =>
  formatQuota(result.value?.auditQuota?.speedupRest, result.value?.auditQuota?.speedupLimit),
)

onMounted(() => {
  void loadAppidOptions()
})
</script>

<template>
  <div class="page">
    <page-header title="前置体检" description="批量作业前逐个小程序检查授权、权限集、类目、隐私指引、域名与额度" />

    <n-card class="section-card" :bordered="true" title="体检配置">
      <n-form label-placement="top">
        <div class="form-grid">
          <n-form-item label="体检目的">
            <n-select v-model:value="purpose" :options="purposeOptions" />
          </n-form-item>
          <n-form-item label="选择方式">
            <n-radio-group v-model:value="selectionMode">
              <n-radio-button value="all">全部已授权小程序</n-radio-button>
              <n-radio-button value="manual">手动勾选 AppID</n-radio-button>
            </n-radio-group>
          </n-form-item>
        </div>

        <n-form-item v-if="selectionMode === 'manual'" label="目标小程序">
          <n-select
            v-model:value="selectedAppids"
            multiple
            filterable
            :options="appidOptions"
            :loading="optionsLoading"
            placeholder="选择要体检的小程序（可搜索）"
          />
        </n-form-item>

        <div class="form-grid">
          <n-form-item label="要求的服务器域名（可选，逗号分隔）">
            <n-input
              v-model:value="requestDomainText"
              placeholder="例如 https://api.example.com,https://cdn.example.com"
            />
          </n-form-item>
          <n-form-item label="要求的业务域名（可选，逗号分隔）">
            <n-input v-model:value="businessDomainText" placeholder="例如 https://m.example.com" />
          </n-form-item>
        </div>
      </n-form>

      <div class="form-actions">
        <n-button type="primary" :loading="loading" @click="runPreflight">开始体检</n-button>
        <n-button :disabled="loading" @click="resetForm">重置</n-button>
      </div>
    </n-card>

    <n-card class="section-card" :bordered="true" title="体检结果">
      <template #header-extra>
        <div class="result-summary">
          <n-tag v-if="result" type="success" size="small" round :bordered="false">
            可执行 {{ result.readyCount }} 个
          </n-tag>
          <n-tag v-if="result" type="error" size="small" round :bordered="false">
            有阻塞 {{ result.blockedCount }} 个
          </n-tag>
          <n-tag size="small" round :bordered="false">提审额度 {{ auditQuotaText }}</n-tag>
          <n-tag size="small" round :bordered="false">加急额度 {{ speedupQuotaText }}</n-tag>
        </div>
      </template>

      <n-data-table
        v-if="result"
        :columns="columns"
        :data="result.items"
        :bordered="false"
        :scroll-x="760"
        :row-key="(row: PreflightItem) => row.appid"
        size="small"
      />
      <div v-else class="empty-hint">
        选择目的与目标小程序后点「开始体检」，结果会在这里逐个小程序列出检查项。
      </div>
    </n-card>
  </div>
</template>

<style scoped>
.section-card {
  margin-bottom: 12px;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 16px;
}

.form-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 4px;
}

.result-summary {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

.empty-hint {
  font-size: 14px;
  line-height: 1.6;
  color: #1f2937;
}

.app-cell {
  min-width: 0;
}

.app-name {
  font-size: 14px;
  font-weight: 600;
  color: #17233d;
  word-break: break-all;
}

.app-id {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 13px;
  color: #1f2937;
  word-break: break-all;
}

.check-wrap {
  padding: 4px 0 8px;
}

.check-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 2px 0;
}

.check-head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.check-label {
  font-size: 14px;
  font-weight: 600;
  color: #17233d;
}

.check-message {
  font-size: 14px;
  line-height: 1.6;
  color: #1f2937;
  word-break: break-all;
}

.check-hint {
  font-size: 14px;
  line-height: 1.6;
  color: #1f2937;
  word-break: break-all;
}

@media (max-width: 820px) {
  .form-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .form-actions :deep(.n-button) {
    width: 100%;
  }

  .result-summary {
    width: 100%;
  }
}
</style>
