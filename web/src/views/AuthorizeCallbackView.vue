<script setup lang="ts">
// 微信授权回调落地页（公开路由 /authorize/callback）：读取 query 的 auth_code，换取令牌并登记小程序。
//   - 未登录：提示先登录（回跳地址保留 auth_code），登录后回到本页自动完成登记；
//   - 已登录：自动 POST /authorizers/authorize；成功展示 appid / 授权状态 / 权限集，
//     缺权限集 18 时展示后端 warnings 警示；失败展示后端消息并提供「重试」。
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NAlert, NButton, NCard, NIcon, NTag, useMessage } from 'naive-ui'
import { CheckmarkCircleOutline, RefreshOutline } from '@vicons/ionicons5'

import { client, errorMessage } from '@/api/client'
import PageHeader from '@/components/PageHeader.vue'
import StatusTag from '@/components/StatusTag.vue'
import type { components } from '@/api/schema'
import { useAuth } from '@/stores/auth'

type AuthorizeResponse = components['schemas']['AuthorizeResponse']

const route = useRoute()
const router = useRouter()
const message = useMessage()
const { isLoggedIn } = useAuth()

const loading = ref(false)
/** 授权登记结果（成功后展示 appid / 授权状态 / 权限集）。 */
const result = ref<AuthorizeResponse | null>(null)
/** 失败原因（后端 message，含微信 errcode 与处置建议）。 */
const errorText = ref('')

/** 从 query 读取 auth_code（兼容微信的 auth_code 与驼峰写法）。 */
const authCode = computed<string>(() => {
  const raw = route.query.auth_code ?? route.query.authCode
  const value = Array.isArray(raw) ? raw[0] : raw
  return typeof value === 'string' ? value.trim() : ''
})

/** 后端返回的警示（缺权限集、需重新授权等）。 */
const warnings = computed<string[]>(() => result.value?.warnings ?? [])

/** 是否缺权限集 18（无法代管代码，需要重新授权勾选）。 */
const missingDevPermission = computed<boolean>(() => result.value?.hasDevPermission === false)

/** 完成授权登记：POST /authorizers/authorize。 */
async function completeAuthorize(): Promise<void> {
  if (authCode.value === '') {
    errorText.value = '未收到 auth_code 参数，请从授权链接或二维码重新进入'
    return
  }
  loading.value = true
  errorText.value = ''
  try {
    const { data, error } = await client.POST('/authorizers/authorize', {
      body: { authCode: authCode.value },
    })
    if (error || !data) {
      errorText.value = errorMessage(error)
      message.error(errorText.value)
      return
    }
    result.value = data
    message.success('授权登记已完成')
  } finally {
    loading.value = false
  }
}

/** 去登录：回跳地址先带上本页（含 auth_code），登录后回来自动登记。 */
function goLogin(): void {
  void router.push({ name: 'login', query: { redirect: route.fullPath } })
}

/** 查看小程序列表。 */
function goAuthorizers(): void {
  void router.push({ name: 'authorizers' })
}

/** 返回概览。 */
function goDashboard(): void {
  void router.push({ name: 'dashboard' })
}

onMounted(() => {
  // 已登录且拿到 auth_code 时自动登记；未登录时等待用户登录后回到本页再触发。
  if (isLoggedIn.value && authCode.value !== '') {
    void completeAuthorize()
  }
})
</script>

<template>
  <div class="page">
    <page-header title="授权回调" description="微信第三方平台授权完成后由微信跳转到本页，自动完成小程序登记" />

    <n-card :bordered="true">
      <!-- 缺少 auth_code：无法登记，提示重新进入 -->
      <template v-if="authCode === ''">
        <n-alert type="error" show-icon>
          未收到 auth_code 参数，请从小程序管理的「生成授权链接」重新发起授权。
        </n-alert>
        <div class="callback-actions">
          <n-button type="primary" @click="goAuthorizers">查看小程序列表</n-button>
          <n-button @click="goDashboard">返回概览</n-button>
        </div>
      </template>

      <!-- 未登录：先登录，回跳本页保留 auth_code -->
      <template v-else-if="!isLoggedIn">
        <n-alert type="warning" show-icon>授权已完成，请先登录平台以完成登记</n-alert>
        <div class="callback-row">
          <span class="callback-label">auth_code</span>
          <span class="callback-value">{{ authCode }}</span>
        </div>
        <div class="callback-actions">
          <n-button type="primary" @click="goLogin">去登录</n-button>
        </div>
      </template>

      <!-- 已登录：自动登记，成功展示结果 -->
      <template v-else>
        <template v-if="result">
          <n-alert v-if="missingDevPermission" type="warning" show-icon>
            <template v-if="warnings.length > 0">
              <div v-for="(item, index) in warnings" :key="index">{{ item }}</div>
            </template>
            <template v-else>
              本次授权未包含权限集 18（小程序开发与数据分析），平台无法代管代码；请让管理员重新授权并单独勾选权限集 18。
            </template>
          </n-alert>

          <div class="callback-row">
            <span class="callback-label">小程序 AppID</span>
            <span class="callback-value mono">{{ result.appid }}</span>
          </div>
          <div class="callback-row">
            <span class="callback-label">授权状态</span>
            <span class="callback-value">
              <status-tag kind="authorizer" :status="result.authorizationStatus" />
            </span>
          </div>
          <div class="callback-row">
            <span class="callback-label">权限集</span>
            <span class="callback-value">
              <n-tag
                v-for="id in result.funcInfoIds"
                :key="id"
                size="small"
                :bordered="false"
                type="info"
              >
                {{ id }}
              </n-tag>
              <span v-if="result.funcInfoIds.length === 0">未授权任何权限集</span>
              <n-tag v-if="missingDevPermission" size="small" type="warning" :bordered="false">
                缺权限集 18，无法代管代码
              </n-tag>
            </span>
          </div>

          <div class="callback-actions">
            <n-button type="primary" @click="goAuthorizers">查看小程序列表</n-button>
            <n-button @click="goDashboard">返回概览</n-button>
          </div>
        </template>

        <template v-else>
          <n-alert v-if="errorText" type="error" show-icon>{{ errorText }}</n-alert>
          <n-alert v-else type="info" show-icon>正在用 auth_code 换取令牌并登记该小程序…</n-alert>
          <div class="callback-row">
            <span class="callback-label">auth_code</span>
            <span class="callback-value">{{ authCode }}</span>
          </div>
          <div class="callback-actions">
            <n-button type="primary" :loading="loading" @click="completeAuthorize">
              <template #icon>
                <n-icon>
                  <refresh-outline v-if="errorText" />
                  <checkmark-circle-outline v-else />
                </n-icon>
              </template>
              {{ errorText ? '重试' : '完成授权' }}
            </n-button>
            <n-button @click="goAuthorizers">查看小程序列表</n-button>
          </div>
        </template>
      </template>
    </n-card>
  </div>
</template>

<style scoped>
.callback-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 0;
  border-bottom: 1px solid #eef2f8;
  flex-wrap: wrap;
}

.callback-row:last-child {
  border-bottom: none;
}

.callback-label {
  width: 140px;
  flex-shrink: 0;
  font-size: 14px;
  font-weight: 600;
  color: #3d4a5c;
}

.callback-value {
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

.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}

.callback-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 16px;
}

@media (max-width: 820px) {
  .callback-label {
    width: 100%;
  }

  .callback-actions :deep(.n-button) {
    width: 100%;
  }
}
</style>
