// 全局组合式状态：登录令牌与当前用户名（localStorage 持久化，无需 Pinia）。
import { computed, ref } from 'vue'

const TOKEN_KEY = 'wx_platform_token'
const USERNAME_KEY = 'wx_platform_username'

const token = ref<string | null>(localStorage.getItem(TOKEN_KEY))
const username = ref<string | null>(localStorage.getItem(USERNAME_KEY))

export function useAuth() {
  const isLoggedIn = computed(() => token.value !== null)

  function setAuth(newToken: string, name: string): void {
    token.value = newToken
    username.value = name
    localStorage.setItem(TOKEN_KEY, newToken)
    localStorage.setItem(USERNAME_KEY, name)
  }

  function clearAuth(): void {
    token.value = null
    username.value = null
    localStorage.removeItem(TOKEN_KEY)
    localStorage.removeItem(USERNAME_KEY)
  }

  return { token, username, isLoggedIn, setAuth, clearAuth }
}