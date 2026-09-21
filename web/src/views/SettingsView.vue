<script setup lang="ts">
// 设置页：第三方平台接入信息（回调 URL / 票据 / 令牌）+ 运行参数（只提交改动过的字段）+ 能力边界说明。
import { computed, onMounted, reactive, ref } from 'vue'
import {
  NAlert,
  NButton,
  NCard,
  NDescriptions,
  NDescriptionsItem,
  NForm,
  NFormItem,
  NGrid,
  NGridItem,
  NIcon,
  NInputNumber,
  NSelect,
  useMessage,
  type FormInst,
  type FormItemRule,
  type FormRules,
} from 'naive-ui'
import { CopyOutline } from '@vicons/ionicons5'

import { client, errorMessage } from '@/api/client'
import PageHeader from '@/components/PageHeader.vue'
import StatusTag from '@/components/StatusTag.vue'
import type { components } from '@/api/schema'
import { EMPTY_TEXT, formatDateTime, formatRelativeTime, usagePercent } from '@/utils/format'

type PlatformStatus = components['schemas']['PlatformStatus']
type Settings = components['schemas']['Settings']
type SettingsUpdateRequest = components['schemas']['SettingsUpdateRequest']

/**
 * 表单模型：数值字段用 `number | null` 表示「输入框被清空」，
 * 可选外键用 `number | null` 表示「不设置默认值」。
 */
interface SettingsForm {
  jobConcurrency: number | null
  jobMaxAttempts: number | null
  wxMaxQps: number | null
  logRetentionDays: number | null
  wxRequestTimeoutSeconds: number | null
  privacyCheckMaxWaitSeconds: number | null
  auditResultMaxWaitSeconds: number | null
  defaultTemplateId: number | null
  defaultAuditProfileId: number | null
}

/** 空表单：字段的兜底值对齐后端启动配置的默认值（后端总会返回这些字段）。 */
const EMPTY_FORM: SettingsForm = {
  jobConcurrency: 3,
  jobMaxAttempts: 3,
  wxMaxQps: 8,
  logRetentionDays: 30,
  wxRequestTimeoutSeconds: 15,
  privacyCheckMaxWaitSeconds: 600,
  auditResultMaxWaitSeconds: 604800,
  defaultTemplateId: null,
  defaultAuditProfileId: null,
}

const message = useMessage()

const loading = ref(false)
const saving = ref(false)
const status = ref<PlatformStatus | null>(null)
const formRef = ref<FormInst | null>(null)
const form = reactive<SettingsForm>({ ...EMPTY_FORM })
/** 服务端返回值归一化后的基线：buildPatch 以它为参照，只提交真正改动过的字段。 */
const baseline = ref<SettingsForm | null>(null)
const templateOptions = ref<{ label: string; value: number }[]>([])

/** 票据状态文案。 */
const ticketLabel = computed<string>(() =>
  status.value?.ticketFresh === true ? '票据正常' : '票据缺失',
)
/** 平台令牌状态文案。 */
const tokenLabel = computed<string>(() =>
  status.value?.componentTokenValid === true ? '平台令牌有效' : '平台令牌未获取',
)
/** 票据最近更新时间（绝对 + 相对）。 */
const ticketTimeText = computed<string>(() => {
  const value = status.value?.ticketUpdatedAt
  if (!value) {
    return EMPTY_TEXT
  }
  return `${formatDateTime(value)}（${formatRelativeTime(value)}）`
})
/** 票据距今天数（秒 → 秒/分钟/小时）。 */
const ticketAgeText = computed<string>(() => {
  const seconds = status.value?.ticketAgeSeconds
  if (seconds === null || seconds === undefined) {
    return EMPTY_TEXT
  }
  if (seconds < 60) {
    return `${seconds} 秒前`
  }
  if (seconds < 3600) {
    return `${Math.floor(seconds / 60)} 分钟前`
  }
  return `${Math.floor(seconds / 3600)} 小时前`
})

/** 数值区间校验器（与后端 checkRange 的范围保持一致）。 */
function rangeRule(label: string, min: number, max: number) {
  return (_rule: FormItemRule, value: number | null | undefined): boolean | Error => {
    if (value === null || value === undefined) {
      return new Error(`请填写${label}`)
    }
    if (value < min || value > max) {
      return new Error(`${label}必须在 ${min}-${max} 之间`)
    }
    return true
  }
}

const rules: FormRules = {
  jobConcurrency: {
    required: true,
    trigger: ['change', 'blur'],
    validator: rangeRule('并发数', 1, 8),
  },
  jobMaxAttempts: {
    required: true,
    trigger: ['change', 'blur'],
    validator: rangeRule('最大重试次数', 1, 8),
  },
  wxMaxQps: { required: true, trigger: ['change', 'blur'], validator: rangeRule('QPS', 1, 50) },
  logRetentionDays: {
    required: true,
    trigger: ['change', 'blur'],
    validator: rangeRule('日志保留天数', 1, 365),
  },
  wxRequestTimeoutSeconds: {
    required: true,
    trigger: ['change', 'blur'],
    validator: rangeRule('请求超时', 1, 120),
  },
  privacyCheckMaxWaitSeconds: {
    required: true,
    trigger: ['change', 'blur'],
    validator: rangeRule('隐私检测最长等待', 10, 3600),
  },
  auditResultMaxWaitSeconds: {
    required: true,
    trigger: ['change', 'blur'],
    validator: rangeRule('审核结果最长等待', 60, 604800),
  },
}

/** 把接口返回的参数归一化进表单模型。 */
function toForm(data: Settings): SettingsForm {
  return {
    jobConcurrency: data.jobConcurrency,
    jobMaxAttempts: data.jobMaxAttempts,
    wxMaxQps: data.wxMaxQps,
    logRetentionDays: data.logRetentionDays,
    wxRequestTimeoutSeconds: data.wxRequestTimeoutSeconds ?? EMPTY_FORM.wxRequestTimeoutSeconds,
    privacyCheckMaxWaitSeconds:
      data.privacyCheckMaxWaitSeconds ?? EMPTY_FORM.privacyCheckMaxWaitSeconds,
    auditResultMaxWaitSeconds:
      data.auditResultMaxWaitSeconds ?? EMPTY_FORM.auditResultMaxWaitSeconds,
    defaultTemplateId: data.defaultTemplateId ?? null,
    defaultAuditProfileId: data.defaultAuditProfileId ?? null,
  }
}

/** 用服务端返回值回填表单，并把它同时作为新的对比基线。 */
function applySettings(data: Settings): void {
  const next = toForm(data)
  Object.assign(form, next)
  baseline.value = { ...next }
}

/**
 * 构造 PUT body：**只放入与基线不同的字段**。
 *
 * 契约 SettingsUpdateRequest 的每个字段都是可选，语义等价于 Go 侧的 *int / *int64：
 * 字段缺失 = 该参数保持不变（后端不会写库）。因此未改动的字段绝不能出现在 body 里，
 * 否则会被后端当成「显式改成这个值」而写库；显式的 null 则表示清空默认模板 / 默认提审资料。
 */
function buildPatch(): SettingsUpdateRequest {
  const base = baseline.value
  const patch: SettingsUpdateRequest = {}
  if (!base) {
    return patch
  }

  if (form.jobConcurrency !== null && form.jobConcurrency !== base.jobConcurrency) {
    patch.jobConcurrency = form.jobConcurrency
  }
  if (form.jobMaxAttempts !== null && form.jobMaxAttempts !== base.jobMaxAttempts) {
    patch.jobMaxAttempts = form.jobMaxAttempts
  }
  if (form.wxMaxQps !== null && form.wxMaxQps !== base.wxMaxQps) {
    patch.wxMaxQps = form.wxMaxQps
  }
  if (form.logRetentionDays !== null && form.logRetentionDays !== base.logRetentionDays) {
    patch.logRetentionDays = form.logRetentionDays
  }
  if (
    form.wxRequestTimeoutSeconds !== null &&
    form.wxRequestTimeoutSeconds !== base.wxRequestTimeoutSeconds
  ) {
    patch.wxRequestTimeoutSeconds = form.wxRequestTimeoutSeconds
  }
  if (
    form.privacyCheckMaxWaitSeconds !== null &&
    form.privacyCheckMaxWaitSeconds !== base.privacyCheckMaxWaitSeconds
  ) {
    patch.privacyCheckMaxWaitSeconds = form.privacyCheckMaxWaitSeconds
  }
  if (
    form.auditResultMaxWaitSeconds !== null &&
    form.auditResultMaxWaitSeconds !== base.auditResultMaxWaitSeconds
  ) {
    patch.auditResultMaxWaitSeconds = form.auditResultMaxWaitSeconds
  }
  // 可空外键：从有值改为「不设置」时提交显式 null（后端会清空该参数）。
  if (form.defaultTemplateId !== base.defaultTemplateId) {
    patch.defaultTemplateId = form.defaultTemplateId
  }
  if (form.defaultAuditProfileId !== base.defaultAuditProfileId) {
    patch.defaultAuditProfileId = form.defaultAuditProfileId
  }

  return patch
}

/** 已改动字段的中文名，用于按钮提示与保存成功提示。 */
const changedLabels = computed<string[]>(() => {
  const base = baseline.value
  if (!base) {
    return []
  }
  const labels: string[] = []
  if (form.jobConcurrency !== null && form.jobConcurrency !== base.jobConcurrency) {
    labels.push('并发数')
  }
  if (form.jobMaxAttempts !== null && form.jobMaxAttempts !== base.jobMaxAttempts) {
    labels.push('最大重试次数')
  }
  if (form.wxMaxQps !== null && form.wxMaxQps !== base.wxMaxQps) {
    labels.push('微信接口 QPS')
  }
  if (form.logRetentionDays !== null && form.logRetentionDays !== base.logRetentionDays) {
    labels.push('日志保留天数')
  }
  if (
    form.wxRequestTimeoutSeconds !== null &&
    form.wxRequestTimeoutSeconds !== base.wxRequestTimeoutSeconds
  ) {
    labels.push('微信请求超时')
  }
  if (
    form.privacyCheckMaxWaitSeconds !== null &&
    form.privacyCheckMaxWaitSeconds !== base.privacyCheckMaxWaitSeconds
  ) {
    labels.push('隐私检测最长等待')
  }
  if (
    form.auditResultMaxWaitSeconds !== null &&
    form.auditResultMaxWaitSeconds !== base.auditResultMaxWaitSeconds
  ) {
    labels.push('审核结果最长等待')
  }
  if (form.defaultTemplateId !== base.defaultTemplateId) {
    labels.push('默认模板')
  }
  if (form.defaultAuditProfileId !== base.defaultAuditProfileId) {
    labels.push('默认提审资料')
  }
  return labels
})

/** 模板库用量文案。 */
const templateUsage = computed<string>(() => {
  const limit = status.value?.templateLimit
  if (limit === null || limit === undefined) {
    return String(status.value?.templateCount ?? 0)
  }
  return `${status.value?.templateCount ?? 0} / ${limit}`
})
/** 模板库用量百分比。 */
const templatePercent = computed<number>(() =>
  usagePercent(status.value?.templateCount, status.value?.templateLimit),
)

/** 平台不提供的能力（每条一句说明）。 */
const boundaries = [
  {
    label: '不代商家处理客服消息',
    text: '客服消息必须由商家在小程序侧自行处理；本平台只接收并留档回调事件，便于排查。',
  },
  {
    label: '不做代注册小程序',
    text: '小程序需由商家自行注册并完成认证，之后再授权给本第三方平台。',
  },
  {
    label: '不支持标准模板',
    text: '官方已下架标准模板（返回 9402203），本平台仅支持普通模板。',
  },
  {
    label: '不支持绕过模板库批量下发代码',
    text: '批量上传代码必须走「草稿箱 → 模板库 → 小程序代码」链路；directCommit 只对单个小程序直接提交，不用于批量。',
  },
  {
    label: '不处理公众号与视频号',
    text: '只处理小程序（需要授权权限集 18：小程序开发与数据分析）。',
  },
  {
    label: '不在数据库中保存平台密钥',
    text: '第三方平台的 AppSecret 只从后端环境变量读取；授权方 refresh_token 以密文入库。',
  },
]

/** 复制文本到剪贴板（未配置时提示而不是复制空串）。 */
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

/** 拉取平台状态、运行参数与普通模板列表。 */
async function loadAll(): Promise<void> {
  loading.value = true
  try {
    const [statusRes, settingsRes, templatesRes] = await Promise.all([
      client.GET('/platform/status'),
      client.GET('/platform/settings'),
      client.GET('/templates', { params: { query: { templateType: 0 } } }),
    ])

    if (statusRes.error || !statusRes.data) {
      message.error(errorMessage(statusRes.error))
    } else {
      status.value = statusRes.data
    }

    if (settingsRes.error || !settingsRes.data) {
      message.error(errorMessage(settingsRes.error))
    } else {
      applySettings(settingsRes.data)
    }

    if (templatesRes.error || !templatesRes.data) {
      templateOptions.value = []
    } else {
      templateOptions.value = templatesRes.data.items.map((item) => ({
        value: item.templateId,
        label: item.userVersion
          ? `#${item.templateId} ${item.userVersion}`
          : `#${item.templateId}`,
      }))
    }
  } finally {
    loading.value = false
  }
}

/** 保存：校验通过后只提交改动过的字段。 */
async function save(): Promise<void> {
  const instance = formRef.value
  if (!instance) {
    return
  }
  try {
    await instance.validate()
  } catch {
    message.warning('请先修正表单中标红的取值')
    return
  }

  const patch = buildPatch()
  const keys = Object.keys(patch)
  if (keys.length === 0) {
    message.info('没有需要保存的改动')
    return
  }

  saving.value = true
  try {
    const res = await client.PUT('/platform/settings', { body: patch })
    if (res.error || !res.data) {
      message.error(errorMessage(res.error))
      return
    }
    applySettings(res.data)
    message.success(`已保存 ${keys.length} 项改动：${keys.join('、')}`)
  } finally {
    saving.value = false
  }
}

onMounted(loadAll)
</script>

<template>
  <div class="page">
    <page-header
      title="设置"
      description="第三方平台接入信息、运行参数与能力边界；运行参数仅提交改动过的字段"
    >
      <template #extra>
        <n-button :loading="loading" @click="loadAll">刷新</n-button>
      </template>
    </page-header>

    <n-card class="section-card" title="平台信息" :bordered="true" :loading="loading">
      <div v-if="status?.warnings && status.warnings.length > 0" class="warning-list">
        <n-alert v-for="(item, index) in status.warnings" :key="index" type="warning" show-icon>
          {{ item }}
        </n-alert>
      </div>

      <n-descriptions bordered :column="1" label-placement="left">
        <n-descriptions-item label="第三方平台 AppID">
          <div class="value-row">
            <span class="value-text">{{ status?.componentAppid ?? EMPTY_TEXT }}</span>
            <n-button size="small" @click="copyText(status?.componentAppid ?? '', '平台 AppID')">
              <template #icon>
                <n-icon><copy-outline /></n-icon>
              </template>
              复制
            </n-button>
          </div>
        </n-descriptions-item>

        <n-descriptions-item label="授权事件接收 URL">
          <div class="value-row">
            <span class="value-text">{{ status?.authorizationEventUrl || '未配置' }}</span>
            <n-button
              size="small"
              @click="copyText(status?.authorizationEventUrl ?? '', '授权事件接收 URL')"
            >
              <template #icon>
                <n-icon><copy-outline /></n-icon>
              </template>
              复制
            </n-button>
          </div>
          <p class="field-note">
            把「授权事件接收 URL」与「消息与事件接收 URL」两个地址填进开放平台后台的「开发配置 → 开发资料」，
            填错会收不到票据（component_verify_ticket）导致所有接口无法调用。
          </p>
        </n-descriptions-item>

        <n-descriptions-item label="消息与事件接收 URL">
          <div class="value-row">
            <span class="value-text">{{ status?.messageEventUrl || '未配置' }}</span>
            <n-button
              size="small"
              @click="copyText(status?.messageEventUrl ?? '', '消息与事件接收 URL')"
            >
              <template #icon>
                <n-icon><copy-outline /></n-icon>
              </template>
              复制
            </n-button>
          </div>
          <p class="field-note">
            该地址含 $APPID$ 占位符，微信会用授权方 appid 替换后回调；同样填进「开发配置 → 开发资料」。
          </p>
        </n-descriptions-item>

        <n-descriptions-item label="授权回调地址">
          <span class="value-text">{{ status?.authRedirectUri || '未配置' }}</span>
        </n-descriptions-item>

        <n-descriptions-item label="票据状态">
          <div class="value-row">
            <status-tag kind="boolean" :status="status?.ticketFresh === true" :label="ticketLabel" />
            <span class="value-text">{{ ticketTimeText }}</span>
            <span class="value-text">距今：{{ ticketAgeText }}</span>
          </div>
          <p class="field-note">
            票据每 10 分钟推送一次；票据缺失时第三方平台的令牌与所有微信接口都会失败。
          </p>
        </n-descriptions-item>

        <n-descriptions-item label="平台令牌">
          <div class="value-row">
            <status-tag
              kind="boolean"
              :status="status?.componentTokenValid === true"
              :label="tokenLabel"
            />
            <span class="value-text">
              有效期至 {{ formatDateTime(status?.componentTokenExpiresAt) }}
            </span>
          </div>
        </n-descriptions-item>

        <n-descriptions-item label="小程序授权情况">
          <span class="value-text">
            已授权 {{ status?.authAuthorized ?? 0 }} 个，已取消授权 {{ status?.authUnauthorized ?? 0 }} 个；
            在途作业 {{ status?.jobRunning ?? 0 }} 个，失败作业 {{ status?.jobFailed ?? 0 }} 个。
          </span>
        </n-descriptions-item>

        <n-descriptions-item label="模板库用量">
          <span class="value-text">{{ templateUsage }}（约 {{ templatePercent }}%）</span>
        </n-descriptions-item>
      </n-descriptions>
    </n-card>

    <n-card class="section-card" title="运行参数" :bordered="true" :loading="loading">
      <n-form
        ref="formRef"
        :model="form"
        :rules="rules"
        label-placement="top"
        :show-require-mark="false"
      >
        <n-grid :x-gap="16" :y-gap="4" cols="1 s:2" responsive="screen">
          <n-grid-item>
            <n-form-item label="并发数" path="jobConcurrency">
              <div class="field-stack">
                <n-input-number
                  v-model:value="form.jobConcurrency"
                  class="full-width"
                  :min="1"
                  :max="8"
                  :precision="0"
                />
                <p class="field-note">
                  同一小程序始终串行；这里控制同时处理多少个小程序（1-8）。
                </p>
              </div>
            </n-form-item>
          </n-grid-item>

          <n-grid-item>
            <n-form-item label="最大重试次数" path="jobMaxAttempts">
              <div class="field-stack">
                <n-input-number
                  v-model:value="form.jobMaxAttempts"
                  class="full-width"
                  :min="1"
                  :max="8"
                  :precision="0"
                />
                <p class="field-note">
                  单个小程序步骤失败后的最大尝试次数（1-8）；只有可重试类错误才消耗次数。
                </p>
              </div>
            </n-form-item>
          </n-grid-item>

          <n-grid-item>
            <n-form-item label="微信接口 QPS" path="wxMaxQps">
              <div class="field-stack">
                <n-input-number
                  v-model:value="form.wxMaxQps"
                  class="full-width"
                  :min="1"
                  :max="50"
                  :precision="0"
                />
                <p class="field-note">
                  全局限速（1-50）；调大可加快批量，调得过高会触发微信侧限频。
                </p>
              </div>
            </n-form-item>
          </n-grid-item>

          <n-grid-item>
            <n-form-item label="日志保留天数" path="logRetentionDays">
              <div class="field-stack">
                <n-input-number
                  v-model:value="form.logRetentionDays"
                  class="full-width"
                  :min="1"
                  :max="365"
                  :precision="0"
                />
                <p class="field-note">超过天数的调用日志与回调事件会被自动清理（1-365）。</p>
              </div>
            </n-form-item>
          </n-grid-item>

          <n-grid-item>
            <n-form-item label="微信请求超时（秒）" path="wxRequestTimeoutSeconds">
              <div class="field-stack">
                <n-input-number
                  v-model:value="form.wxRequestTimeoutSeconds"
                  class="full-width"
                  :min="1"
                  :max="120"
                  :precision="0"
                />
                <p class="field-note">单个微信 HTTP 请求的超时秒数（1-120），超时按可重试处理。</p>
              </div>
            </n-form-item>
          </n-grid-item>

          <n-grid-item>
            <n-form-item label="隐私检测最长等待（秒）" path="privacyCheckMaxWaitSeconds">
              <div class="field-stack">
                <n-input-number
                  v-model:value="form.privacyCheckMaxWaitSeconds"
                  class="full-width"
                  :min="10"
                  :max="3600"
                  :precision="0"
                />
                <p class="field-note">
                  上传代码后等待隐私检测结束的最长秒数（10-3600）；未结束就提审会返回 61039。
                </p>
              </div>
            </n-form-item>
          </n-grid-item>

          <n-grid-item>
            <n-form-item label="审核结果最长等待（秒）" path="auditResultMaxWaitSeconds">
              <div class="field-stack">
                <n-input-number
                  v-model:value="form.auditResultMaxWaitSeconds"
                  class="full-width"
                  :min="60"
                  :max="604800"
                  :precision="0"
                />
                <p class="field-note">
                  提审后等待审核结果的最长秒数（60-604800，即最多 7 天）；到点仍未出结果会暂挂作业。
                </p>
              </div>
            </n-form-item>
          </n-grid-item>

          <n-grid-item>
            <n-form-item label="默认模板">
              <div class="field-stack">
                <n-select
                  v-model:value="form.defaultTemplateId"
                  class="full-width"
                  :options="templateOptions"
                  placeholder="不指定默认模板"
                  clearable
                  filterable
                />
                <p class="field-note">
                  批量上传代码未显式指定模板时使用该模板；只列普通模板（标准模板官方已下架）。清空表示不设置。
                </p>
              </div>
            </n-form-item>
          </n-grid-item>

          <n-grid-item>
            <n-form-item label="默认提审资料 ID">
              <div class="field-stack">
                <n-input-number
                  v-model:value="form.defaultAuditProfileId"
                  class="full-width"
                  :min="1"
                  :precision="0"
                  placeholder="不设置默认资料"
                  clearable
                />
                <p class="field-note">
                  提审时未单独指定资料则使用该资料 ID；留空表示不设置默认资料。
                </p>
              </div>
            </n-form-item>
          </n-grid-item>
        </n-grid>
      </n-form>

      <div class="save-row">
        <n-button type="primary" :loading="saving" :disabled="changedLabels.length === 0" @click="save">
          保存
        </n-button>
        <span class="value-text">
          <template v-if="changedLabels.length === 0">
            当前没有任何改动；保存时只提交改动过的字段，未改字段不会写库。
          </template>
          <template v-else>
            将提交 {{ changedLabels.length }} 项改动：{{ changedLabels.join('、') }}。
          </template>
        </span>
      </div>
    </n-card>

    <n-card title="本平台不做什么" :bordered="true">
      <n-descriptions bordered :column="1" label-placement="left">
        <n-descriptions-item v-for="item in boundaries" :key="item.label" :label="item.label">
          <span class="value-text">{{ item.text }}</span>
        </n-descriptions-item>
      </n-descriptions>
    </n-card>
  </div>
</template>

<style scoped>
.section-card {
  margin-bottom: 12px;
}

.warning-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 12px;
}

.value-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

/* 说明与取值一律用正文色 14px，不使用灰色小字。 */
.value-text {
  font-size: 14px;
  color: #1f2937;
  word-break: break-all;
}

.field-stack {
  width: 100%;
}

.field-note {
  margin: 6px 0 0;
  font-size: 14px;
  line-height: 1.6;
  color: #3d4a5c;
}

.full-width {
  width: 100%;
}

.save-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 8px;
  padding-top: 12px;
  border-top: 1px solid #eef2f8;
}

@media (max-width: 820px) {
  .save-row :deep(.n-button) {
    width: 100%;
  }
}
</style>
