<script setup lang="ts">
// 概览页：接入自检告警 + 平台指标卡 + 接入配置（回调 URL/票据/令牌）+ 最近作业 + 快捷入口。
import { computed, h, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NGrid,
  NGridItem,
  NIcon,
  NProgress,
  useMessage,
  type DataTableColumns,
} from 'naive-ui'
import {
  AddCircleOutline,
  AppsOutline,
  CloudUploadOutline,
  CopyOutline,
  PulseOutline,
} from '@vicons/ionicons5'

import { client, errorMessage } from '@/api/client'
import StatusTag from '@/components/StatusTag.vue'
import type { components } from '@/api/schema'
import {
  EMPTY_TEXT,
  formatDateTime,
  formatQuota,
  formatRelativeTime,
  jobTypeText,
  truncateMiddle,
  usagePercent,
} from '@/utils/format'

type PlatformStatus = components['schemas']['PlatformStatus']
type Job = components['schemas']['Job']

/** 指标卡模型。 */
interface MetricCard {
  key: string
  label: string
  value: string
  /** 模板库用量进度条百分比；为 null 时不渲染进度条。 */
  percent: number | null
}

const router = useRouter()
const message = useMessage()

const loading = ref(false)
const platform = ref<PlatformStatus | null>(null)
const recentJobs = ref<Job[]>([])

/** 接入自检告警（票据未收到、凭据未配置、已取消授权的小程序等）。 */
const warnings = computed<string[]>(() => platform.value?.warnings ?? [])

/** 指标卡数据。 */
const metrics = computed<MetricCard[]>(() => {
  const status = platform.value
  if (!status) {
    return []
  }
  const templateLimit = status.templateLimit ?? null
  return [
    { key: 'authorized', label: '已授权', value: String(status.authAuthorized), percent: null },
    { key: 'unauthorized', label: '已取消授权', value: String(status.authUnauthorized), percent: null },
    { key: 'running', label: '在途作业', value: String(status.jobRunning), percent: null },
    { key: 'failed', label: '失败作业', value: String(status.jobFailed ?? 0), percent: null },
    {
      key: 'templates',
      label: '模板库用量',
      value: formatQuota(status.templateCount, templateLimit),
      percent: templateLimit === null ? null : usagePercent(status.templateCount, templateLimit),
    },
    {
      key: 'auditQuota',
      label: '提审额度剩余',
      value: formatQuota(status.auditQuota?.rest, status.auditQuota?.limit),
      percent: null,
    },
    {
      key: 'speedupQuota',
      label: '加急额度剩余',
      value: formatQuota(status.auditQuota?.speedupRest, status.auditQuota?.speedupLimit),
      percent: null,
    },
  ]
})

/** 微信票据状态文案。 */
const ticketLabel = computed<string>(() =>
  platform.value?.ticketFresh === true ? '票据正常' : '票据缺失',
)
/** 第三方平台令牌状态文案。 */
const tokenLabel = computed<string>(() =>
  platform.value?.componentTokenValid === true ? '令牌有效' : '令牌未获取',
)
/** 最近票据时间（相对时间 + 绝对时间提示）。 */
const ticketTimeText = computed<string>(() => {
  const value = platform.value?.ticketUpdatedAt
  if (!value) {
    return EMPTY_TEXT
  }
  return `${formatDateTime(value)}（${formatRelativeTime(value)}）`
})

/** 最近作业表格列定义。 */
const jobColumns: DataTableColumns<Job> = [
  { title: '作业 ID', key: 'id', width: 150, render: (row) => truncateMiddle(row.id, 8, 4) },
  { title: '类型', key: 'type', width: 130, render: (row) => jobTypeText(row.type) },
  {
    title: '状态',
    key: 'status',
    width: 110,
    render: (row) => h(StatusTag, { kind: 'job', status: row.status }),
  },
  { title: '成功', key: 'succeeded', width: 80 },
  { title: '失败', key: 'failed', width: 80 },
  {
    title: '创建时间',
    key: 'createdAt',
    width: 170,
    render: (row) => formatDateTime(row.createdAt),
  },
]

/** 行点击进入作业详情。 */
function jobRowProps(row: Job) {
  return {
    style: 'cursor: pointer;',
    onClick: () => {
      void router.push({ name: 'job-detail', params: { id: row.id } })
    },
  }
}

/** 复制文本到剪贴板（未配置的地址给出提示而不是复制空串）。 */
async function copyText(text: string, label: string): Promise<void> {
  if (text.trim() === '') {
    message.warning(`${label}尚未配置`)
    return
  }
  try {
    await navigator.clipboard.writeText(text)
    message.success(`${label}已复制`)
  } catch {
    message.error('浏览器拒绝访问剪贴板，请手动选择复制')
  }
}

/** 跳转快捷入口。 */
function goTo(name: string): void {
  void router.push({ name })
}

/** 加载平台状态与最近 5 条作业。 */
async function loadData(): Promise<void> {
  loading.value = true
  try {
    const [statusRes, jobsRes] = await Promise.all([
      client.GET('/platform/status'),
      client.GET('/jobs', { params: { query: { pageSize: 5 } } }),
    ])
    if (statusRes.error || !statusRes.data) {
      message.error(errorMessage(statusRes.error))
    } else {
      platform.value = statusRes.data
    }
    if (jobsRes.error || !jobsRes.data) {
      message.error(errorMessage(jobsRes.error))
    } else {
      recentJobs.value = jobsRes.data.items
    }
  } finally {
    loading.value = false
  }
}

onMounted(loadData)
</script>

<template>
  <div class="page">
    <!-- 接入自检告警：票据/凭据/权限集等问题逐条展示 -->
    <div v-if="warnings.length > 0" class="warning-list">
      <n-alert v-for="(item, index) in warnings" :key="index" type="warning" show-icon>
        {{ item }}
      </n-alert>
    </div>

    <n-grid :x-gap="12" :y-gap="12" cols="1 s:2 m:4" responsive="screen" class="metric-grid">
      <n-grid-item v-for="item in metrics" :key="item.key">
        <n-card class="metric-card" :bordered="true" size="small">
          <div class="metric-label">{{ item.label }}</div>
          <div class="metric-value">{{ item.value }}</div>
          <n-progress
            v-if="item.percent !== null"
            class="metric-progress"
            type="line"
            :percentage="item.percent"
            :show-indicator="false"
            :height="6"
          />
        </n-card>
      </n-grid-item>
    </n-grid>

    <n-card class="section-card" title="接入配置" :bordered="true">
      <div class="config-row">
        <div class="config-label">第三方平台 AppID</div>
        <div class="config-value">{{ platform?.componentAppidMasked ?? platform?.componentAppid ?? EMPTY_TEXT }}</div>
        <div class="config-action" />
      </div>

      <div class="config-row">
        <div class="config-label">授权事件接收 URL</div>
        <div class="config-value">{{ platform?.authorizationEventUrl || '未配置' }}</div>
        <div class="config-action">
          <n-button size="small" @click="copyText(platform?.authorizationEventUrl ?? '', '授权事件接收 URL')">
            <template #icon>
              <n-icon><copy-outline /></n-icon>
            </template>
            复制
          </n-button>
        </div>
      </div>

      <div class="config-row">
        <div class="config-label">消息与事件接收 URL</div>
        <div class="config-value">{{ platform?.messageEventUrl || '未配置' }}</div>
        <div class="config-action">
          <n-button size="small" @click="copyText(platform?.messageEventUrl ?? '', '消息与事件接收 URL')">
            <template #icon>
              <n-icon><copy-outline /></n-icon>
            </template>
            复制
          </n-button>
        </div>
      </div>

      <div class="config-row">
        <div class="config-label">票据状态</div>
        <div class="config-value">
          <status-tag kind="boolean" :status="platform?.ticketFresh === true" :label="ticketLabel" />
          <span class="config-text">{{ ticketTimeText }}</span>
        </div>
        <div class="config-action" />
      </div>

      <div class="config-row">
        <div class="config-label">平台令牌</div>
        <div class="config-value">
          <status-tag kind="boolean" :status="platform?.componentTokenValid === true" :label="tokenLabel" />
          <span class="config-text">有效期至 {{ formatDateTime(platform?.componentTokenExpiresAt) }}</span>
        </div>
        <div class="config-action" />
      </div>
    </n-card>

    <n-card class="section-card" title="最近作业" :bordered="true">
      <template #header-extra>
        <n-button size="small" @click="goTo('jobs')">查看全部</n-button>
      </template>
      <n-data-table
        :columns="jobColumns"
        :data="recentJobs"
        :loading="loading"
        :bordered="false"
        :row-props="jobRowProps"
        :scroll-x="720"
        size="small"
      />
    </n-card>

    <n-card class="section-card" title="快捷入口" :bordered="true">
      <div class="quick-actions">
        <n-button type="primary" @click="goTo('authorizers')">
          <template #icon>
            <n-icon><apps-outline /></n-icon>
          </template>
          生成授权链接
        </n-button>
        <n-button type="primary" secondary @click="goTo('templates')">
          <template #icon>
            <n-icon><cloud-upload-outline /></n-icon>
          </template>
          同步模板
        </n-button>
        <n-button type="primary" secondary @click="goTo('job-create')">
          <template #icon>
            <n-icon><add-circle-outline /></n-icon>
          </template>
          新建批量任务
        </n-button>
        <n-button type="primary" secondary @click="goTo('preflight')">
          <template #icon>
            <n-icon><pulse-outline /></n-icon>
          </template>
          前置体检
        </n-button>
      </div>
    </n-card>
  </div>
</template>

<style scoped>
.warning-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 12px;
}

.metric-grid {
  margin-bottom: 12px;
}

.metric-card {
  height: 100%;
}

.metric-label {
  font-size: 14px;
  font-weight: 600;
  color: #3d4a5c;
}

.metric-value {
  margin-top: 6px;
  font-size: 24px;
  font-weight: 650;
  color: #17233d;
  word-break: break-all;
}

.metric-progress {
  margin-top: 8px;
}

.section-card {
  margin-bottom: 12px;
}

.config-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 0;
  border-bottom: 1px solid #eef2f8;
  flex-wrap: wrap;
}

.config-row:last-child {
  border-bottom: none;
}

.config-label {
  width: 160px;
  flex-shrink: 0;
  font-size: 14px;
  font-weight: 600;
  color: #3d4a5c;
}

.config-value {
  flex: 1;
  min-width: 200px;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 14px;
  color: #1f2937;
  word-break: break-all;
}

.config-text {
  font-size: 14px;
  color: #1f2937;
}

.config-action {
  flex-shrink: 0;
}

.quick-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

@media (max-width: 820px) {
  .config-label {
    width: 100%;
  }

  .config-action {
    width: 100%;
  }

  /* 窄屏下快捷入口按钮占满整行 */
  .quick-actions :deep(.n-button) {
    width: 100%;
  }
}
</style>
