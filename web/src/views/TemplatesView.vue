<script setup lang="ts">
// 代码模板库：草稿箱（微信侧草稿 → 加入模板库）+ 模板库（默认模板 / 备注 / 删除 / 用量上限）。
// 关键约定（见 api/openapi.yaml）：
//   1. POST /drafts/sync、POST /templates/sync 返回 SyncSummary（total/succeeded/failed）；
//   2. POST /drafts/{draftId}/add-to-template 的 200 响应体是「同步后的模板列表」，直接拿来刷新模板表；
//   3. 模板库上限 200（官方限制），模板类型 0=普通模板（唯一可用）、1=标准模板（官方已下架）。
import { computed, h, onMounted, ref } from 'vue'
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NEllipsis,
  NIcon,
  NInput,
  NModal,
  NProgress,
  NTabPane,
  NTabs,
  NTag,
  useDialog,
  useMessage,
  type DataTableColumns,
} from 'naive-ui'
import { RefreshOutline } from '@vicons/ionicons5'

import { client, errorErrcode, errorMessage } from '@/api/client'
import type { components } from '@/api/schema'
import PageHeader from '@/components/PageHeader.vue'
import { EMPTY_TEXT, formatDateTime, usagePercent } from '@/utils/format'

type CodeDraft = components['schemas']['CodeDraft']
type CodeTemplate = components['schemas']['CodeTemplate']
type TemplateListResponse = components['schemas']['TemplateListResponse']

const message = useMessage()
const dialog = useDialog()

const activeTab = ref<'drafts' | 'templates'>('drafts')

const drafts = ref<CodeDraft[]>([])
const draftsLoading = ref(false)
const draftsSyncing = ref(false)
/** 正在「添加到模板库」的 draftId，用于按钮 loading。 */
const addingDraftId = ref<number | null>(null)

const templates = ref<CodeTemplate[]>([])
/** 模板库上限，由 GET /templates 的 limit 字段返回（官方 200）。 */
const templateLimit = ref(0)
const templatesLoading = ref(false)
const templatesSyncing = ref(false)
/** 正在「设为默认」的 templateId。 */
const updatingTemplateId = ref<number | null>(null)

const showNoteModal = ref(false)
const noteTarget = ref<CodeTemplate | null>(null)
const noteText = ref('')
const noteSaving = ref(false)

/** 模板库用量百分比。 */
const usage = computed(() => usagePercent(templates.value.length, templateLimit.value))
/** 用量文案：`已用 / 上限`。 */
const usageText = computed(() =>
  templateLimit.value > 0
    ? `${templates.value.length} / ${templateLimit.value}`
    : String(templates.value.length),
)
/** 接近上限（≥90%）时提示清理。 */
const nearLimit = computed(() => templateLimit.value > 0 && usage.value >= 90)

/** 微信 createTime 为秒级时间戳，转成 RFC3339 后复用统一的 formatDateTime。 */
function formatUnixSeconds(value: number | null | undefined): string {
  if (value === null || value === undefined || value <= 0) {
    return EMPTY_TEXT
  }
  return formatDateTime(new Date(value * 1000).toISOString())
}

/** 错误文案：微信侧失败（502）带上 errcode 与中文说明。 */
function describeError(err: unknown): string {
  const code = errorErrcode(err)
  const text = errorMessage(err)
  return code === null ? text : `${text}（微信 errcode ${code}）`
}

/** 草稿箱表格列。 */
const draftColumns: DataTableColumns<CodeDraft> = [
  { title: '草稿 ID', key: 'draftId', width: 110 },
  {
    title: '版本号',
    key: 'userVersion',
    width: 160,
    render: (row) =>
      h(NEllipsis, { style: 'max-width: 150px' }, { default: () => row.userVersion ?? EMPTY_TEXT }),
  },
  {
    title: '版本描述',
    key: 'userDesc',
    width: 200,
    render: (row) => h(NEllipsis, { style: 'max-width: 190px' }, { default: () => row.userDesc ?? EMPTY_TEXT }),
  },
  {
    title: '开发小程序',
    key: 'sourceMiniProgram',
    width: 180,
    render: (row) => row.sourceMiniProgram ?? EMPTY_TEXT,
  },
  {
    title: '开发小程序 AppID',
    key: 'sourceMiniProgramAppid',
    width: 200,
    render: (row) => row.sourceMiniProgramAppid ?? EMPTY_TEXT,
  },
  { title: '开发者', key: 'developer', width: 130, render: (row) => row.developer ?? EMPTY_TEXT },
  {
    title: '创建时间',
    key: 'createTime',
    width: 170,
    render: (row) => formatUnixSeconds(row.createTime),
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
          type: 'primary',
          secondary: true,
          loading: addingDraftId.value === row.draftId,
          onClick: () => void addToTemplate(row),
        },
        { default: () => '添加到模板库' },
      ),
  },
]

/** 模板库表格列。 */
const templateColumns: DataTableColumns<CodeTemplate> = [
  { title: '模板 ID', key: 'templateId', width: 110 },
  {
    title: '版本号',
    key: 'userVersion',
    width: 160,
    render: (row) =>
      h(NEllipsis, { style: 'max-width: 150px' }, { default: () => row.userVersion ?? EMPTY_TEXT }),
  },
  {
    title: '版本描述',
    key: 'userDesc',
    width: 200,
    render: (row) => h(NEllipsis, { style: 'max-width: 190px' }, { default: () => row.userDesc ?? EMPTY_TEXT }),
  },
  {
    title: '来源小程序',
    key: 'sourceMiniProgram',
    width: 180,
    render: (row) => row.sourceMiniProgram ?? EMPTY_TEXT,
  },
  {
    title: '类型',
    key: 'templateType',
    width: 260,
    render: (row) =>
      row.templateType === 1
        ? h(
            NTag,
            { type: 'warning', size: 'small', round: true, bordered: false },
            { default: () => '标准模板：官方已下架，不可用于下发' },
          )
        : h(NTag, { size: 'small', round: true, bordered: false }, { default: () => '普通模板' }),
  },
  {
    title: '创建时间',
    key: 'createTime',
    width: 170,
    render: (row) => formatUnixSeconds(row.createTime),
  },
  {
    title: '默认',
    key: 'isDefault',
    width: 110,
    render: (row) =>
      row.isDefault
        ? h(NTag, { type: 'success', size: 'small', round: true, bordered: false }, { default: () => '默认模板' })
        : EMPTY_TEXT,
  },
  {
    title: '备注',
    key: 'note',
    width: 200,
    render: (row) => h(NEllipsis, { style: 'max-width: 190px' }, { default: () => row.note ?? EMPTY_TEXT }),
  },
  {
    title: '操作',
    key: 'actions',
    width: 260,
    fixed: 'right',
    render: (row) =>
      h('div', { class: 'row-actions' }, [
        h(
          NButton,
          {
            size: 'small',
            secondary: true,
            disabled: row.isDefault || row.templateType === 1,
            loading: updatingTemplateId.value === row.templateId,
            onClick: () => void setAsDefault(row),
          },
          { default: () => '设为默认' },
        ),
        h(NButton, { size: 'small', secondary: true, onClick: () => openNoteModal(row) }, { default: () => '备注' }),
        h(
          NButton,
          {
            size: 'small',
            type: 'error',
            secondary: true,
            onClick: () => confirmDelete(row),
          },
          { default: () => '删除' },
        ),
      ]),
  },
]

/** 用后端返回的模板列表刷新模板表与用量。 */
function applyTemplates(payload: TemplateListResponse): void {
  templates.value = payload.items
  templateLimit.value = payload.limit
}

/** 加载草稿箱。 */
async function loadDrafts(): Promise<void> {
  draftsLoading.value = true
  try {
    const res = await client.GET('/drafts')
    if (res.error || !res.data) {
      message.error(errorMessage(res.error))
      return
    }
    drafts.value = res.data.items
  } finally {
    draftsLoading.value = false
  }
}

/** 加载模板库。 */
async function loadTemplates(): Promise<void> {
  templatesLoading.value = true
  try {
    const res = await client.GET('/templates')
    if (res.error || !res.data) {
      message.error(errorMessage(res.error))
      return
    }
    applyTemplates(res.data)
  } finally {
    templatesLoading.value = false
  }
}

/** 同步草稿箱（拉取微信侧最新草稿）。 */
async function syncDrafts(): Promise<void> {
  draftsSyncing.value = true
  try {
    const res = await client.POST('/drafts/sync')
    if (res.error || !res.data) {
      message.error(describeError(res.error))
      return
    }
    const summary = res.data
    const text = `草稿箱同步完成：共 ${summary.total} 条，成功 ${summary.succeeded} 条，失败 ${summary.failed} 条`
    if (summary.failed > 0) {
      message.warning(text)
    } else {
      message.success(text)
    }
    await loadDrafts()
  } finally {
    draftsSyncing.value = false
  }
}

/** 同步模板库。 */
async function syncTemplates(): Promise<void> {
  templatesSyncing.value = true
  try {
    const res = await client.POST('/templates/sync')
    if (res.error || !res.data) {
      message.error(describeError(res.error))
      return
    }
    const summary = res.data
    const text = `模板库同步完成：共 ${summary.total} 条，成功 ${summary.succeeded} 条，失败 ${summary.failed} 条`
    if (summary.failed > 0) {
      message.warning(text)
    } else {
      message.success(text)
    }
    await loadTemplates()
  } finally {
    templatesSyncing.value = false
  }
}

/** 把草稿加入模板库：成功后响应体即最新的模板列表。 */
async function addToTemplate(row: CodeDraft): Promise<void> {
  addingDraftId.value = row.draftId
  try {
    const res = await client.POST('/drafts/{draftId}/add-to-template', {
      params: { path: { draftId: row.draftId } },
      body: { templateType: 0 },
    })
    if (res.error || !res.data) {
      message.error(describeError(res.error))
      return
    }
    applyTemplates(res.data)
    activeTab.value = 'templates'
    message.success('已加入模板库，模板 ID 见列表')
  } finally {
    addingDraftId.value = null
  }
}

/** 设为默认模板。 */
async function setAsDefault(row: CodeTemplate): Promise<void> {
  updatingTemplateId.value = row.templateId
  try {
    const res = await client.PATCH('/templates/{templateId}', {
      params: { path: { templateId: row.templateId } },
      body: { isDefault: true },
    })
    if (res.error || !res.data) {
      message.error(describeError(res.error))
      return
    }
    message.success(`模板 ${row.templateId} 已设为默认`)
    await loadTemplates()
  } finally {
    updatingTemplateId.value = null
  }
}

/** 打开备注编辑弹窗。 */
function openNoteModal(row: CodeTemplate): void {
  noteTarget.value = row
  noteText.value = row.note ?? ''
  showNoteModal.value = true
}

/** 保存备注。 */
async function saveNote(): Promise<void> {
  const target = noteTarget.value
  if (!target) {
    return
  }
  noteSaving.value = true
  try {
    const text = noteText.value.trim()
    const res = await client.PATCH('/templates/{templateId}', {
      params: { path: { templateId: target.templateId } },
      body: { note: text === '' ? null : text },
    })
    if (res.error || !res.data) {
      message.error(describeError(res.error))
      return
    }
    showNoteModal.value = false
    message.success('备注已保存')
    await loadTemplates()
  } finally {
    noteSaving.value = false
  }
}

/** 删除模板（二次确认；后端 502 时展示微信 errcode 与中文说明）。 */
function confirmDelete(row: CodeTemplate): void {
  dialog.warning({
    title: '删除模板',
    content: `确定删除模板 ${row.templateId}（${row.userVersion ?? '无版本号'}）？删除后该模板不可再用于下发代码。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      await removeTemplate(row)
    },
  })
}

/** 调用 DELETE 删除模板。 */
async function removeTemplate(row: CodeTemplate): Promise<void> {
  const res = await client.DELETE('/templates/{templateId}', {
    params: { path: { templateId: row.templateId } },
  })
  if (res.error) {
    message.error(describeError(res.error))
    return
  }
  message.success(`模板 ${row.templateId} 已删除`)
  await loadTemplates()
}

onMounted(async () => {
  await Promise.all([loadDrafts(), loadTemplates()])
})
</script>

<template>
  <div class="page">
    <page-header title="代码模板" description="草稿箱与模板库同步、模板维护（模板库上限 200，标准模板已下架）" />

    <n-card :bordered="true">
      <n-tabs v-model:value="activeTab" type="line" animated>
        <n-tab-pane name="drafts" tab="草稿箱">
          <div class="toolbar">
            <n-button type="primary" secondary :loading="draftsSyncing" @click="syncDrafts">
              <template #icon>
                <n-icon><refresh-outline /></n-icon>
              </template>
              同步草稿箱
            </n-button>
            <n-button secondary :loading="draftsLoading" @click="loadDrafts">刷新列表</n-button>
            <span class="toolbar-text">共 {{ drafts.length }} 条草稿</span>
          </div>
          <n-data-table
            :columns="draftColumns"
            :data="drafts"
            :loading="draftsLoading"
            :bordered="false"
            :scroll-x="1300"
            :row-key="(row: CodeDraft) => row.draftId"
            size="small"
          />
        </n-tab-pane>

        <n-tab-pane name="templates" tab="模板库">
          <div class="toolbar">
            <n-button type="primary" secondary :loading="templatesSyncing" @click="syncTemplates">
              <template #icon>
                <n-icon><refresh-outline /></n-icon>
              </template>
              同步模板库
            </n-button>
            <n-button secondary :loading="templatesLoading" @click="loadTemplates">刷新列表</n-button>
            <span class="toolbar-text">模板库用量 {{ usageText }}</span>
          </div>

          <div class="usage">
            <n-progress
              type="line"
              :percentage="usage"
              :height="8"
              :show-indicator="false"
              :status="nearLimit ? 'warning' : 'default'"
            />
          </div>

          <n-alert v-if="nearLimit" type="warning" show-icon class="usage-alert">
            模板库上限 {{ templateLimit }} 条，请先删除不用的模板。
          </n-alert>

          <n-data-table
            :columns="templateColumns"
            :data="templates"
            :loading="templatesLoading"
            :bordered="false"
            :scroll-x="1520"
            :row-key="(row: CodeTemplate) => row.templateId"
            size="small"
          />
        </n-tab-pane>
      </n-tabs>
    </n-card>

    <n-modal
      v-model:show="showNoteModal"
      preset="card"
      title="编辑模板备注"
      class="note-modal"
      :style="{ maxWidth: '94vw', width: '520px' }"
    >
      <div class="note-form">
        <div class="note-label">模板 ID</div>
        <div class="note-value">{{ noteTarget?.templateId ?? EMPTY_TEXT }}</div>
        <div class="note-label">备注</div>
        <n-input
          v-model:value="noteText"
          type="textarea"
          :rows="3"
          maxlength="200"
          show-count
          placeholder="例如：主商城模板，2025 春季活动用"
        />
      </div>
      <template #footer>
        <div class="modal-footer">
          <n-button secondary @click="showNoteModal = false">取消</n-button>
          <n-button type="primary" :loading="noteSaving" @click="saveNote">保存备注</n-button>
        </div>
      </template>
    </n-modal>
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

.toolbar-text {
  font-size: 14px;
  color: #1f2937;
}

.usage {
  max-width: 520px;
  margin-bottom: 12px;
}

.usage-alert {
  margin-bottom: 12px;
}

.row-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.note-form {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.note-label {
  font-size: 14px;
  font-weight: 600;
  color: #3d4a5c;
}

.note-value {
  font-size: 14px;
  color: #1f2937;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

@media (max-width: 820px) {
  /* 窄屏工具条控件占满整行 */
  .toolbar :deep(.n-button) {
    width: 100%;
  }

  .usage {
    max-width: 100%;
  }

  .modal-footer {
    flex-direction: column-reverse;
  }

  .modal-footer :deep(.n-button) {
    width: 100%;
  }
}
</style>
