<script setup lang="ts">
// 登录页：居中卡片 + 表单校验 + 回车提交；成功后写入会话并按 redirect 参数回跳。
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  NButton,
  NForm,
  NFormItem,
  NIcon,
  NInput,
  useMessage,
  type FormInst,
  type FormRules,
} from 'naive-ui'
import { AppsOutline } from '@vicons/ionicons5'

import { client, errorMessage } from '@/api/client'
import { useAuth } from '@/stores/auth'

/** 登录表单模型。 */
interface LoginForm {
  username: string
  password: string
}

const router = useRouter()
const route = useRoute()
const message = useMessage()
const { setAuth } = useAuth()

const formRef = ref<FormInst | null>(null)
const loading = ref(false)
/** 平台账号固定为 admin，此处预填减少输入。 */
const form = ref<LoginForm>({ username: 'admin', password: '' })

const rules: FormRules = {
  username: { required: true, message: '请输入账号', trigger: ['input', 'blur'] },
  password: { required: true, message: '请输入密码', trigger: ['input', 'blur'] },
}

/** 提交登录：校验 → 调接口 → 写入会话 → 跳转。 */
async function handleSubmit(): Promise<void> {
  loading.value = true
  try {
    try {
      await formRef.value?.validate()
    } catch {
      return // 前端校验未通过
    }
    const { data, error } = await client.POST('/auth/login', {
      body: { username: form.value.username.trim(), password: form.value.password },
    })
    if (error || !data) {
      message.error(errorMessage(error))
      return
    }
    setAuth(data.token, data.user.username)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/dashboard'
    await router.replace(redirect)
    message.success('登录成功')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <div class="login-card">
      <div class="login-head">
        <div class="login-logo">
          <n-icon :size="30" color="#1668dc">
            <apps-outline />
          </n-icon>
        </div>
        <h1 class="login-title">微信第三方平台管理平台</h1>
      </div>
      <n-form ref="formRef" :model="form" :rules="rules" size="large" @keyup.enter="handleSubmit">
        <n-form-item label="账号" path="username">
          <n-input v-model:value="form.username" placeholder="请输入账号" />
        </n-form-item>
        <n-form-item label="密码" path="password">
          <n-input
            v-model:value="form.password"
            type="password"
            show-password-on="mousedown"
            placeholder="请输入密码"
          />
        </n-form-item>
        <n-button class="login-btn" type="primary" block :loading="loading" @click="handleSubmit">
          登 录
        </n-button>
      </n-form>
      <p class="login-note">平台账号为 admin，密码由服务端 ADMIN_PASSWORD 决定</p>
    </div>
  </div>
</template>

<style scoped>
.login-page {
  min-height: 100vh;
  min-height: 100dvh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px 0;
  background:
    radial-gradient(1200px 500px at 20% -10%, rgba(64, 152, 252, 0.35), transparent 60%),
    radial-gradient(1000px 500px at 90% 110%, rgba(22, 104, 220, 0.3), transparent 55%),
    #eef4fb;
}

.login-card {
  width: min(380px, calc(100vw - 32px));
  padding: 40px 36px 32px;
  background: #fff;
  border-radius: 12px;
  box-shadow: 0 12px 40px rgba(22, 104, 220, 0.12);
  border: 1px solid #dbe5f1;
}

@media (max-width: 820px) {
  .login-card {
    padding: 28px 20px 24px;
  }
}

.login-head {
  text-align: center;
  margin-bottom: 28px;
}

.login-logo {
  width: 52px;
  height: 52px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0 auto 12px;
  border-radius: 12px;
  background: #d9e8fb;
}

.login-title {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  color: #17233d;
}

.login-btn {
  margin-top: 8px;
}

/* 说明使用正文色 14px，不使用灰色小字。 */
.login-note {
  margin: 20px 0 0;
  text-align: center;
  font-size: 14px;
  line-height: 1.5;
  color: #1f2937;
}
</style>
