// 响应式断点组合式：基于 matchMedia 的移动端判定。
import { ref, type Ref } from 'vue'

/** 移动端（抽屉式侧栏）最大宽度，与样式中媒体查询保持一致。 */
export const MOBILE_MAX_WIDTH = 820

const cache = new Map<string, Ref<boolean>>()

/**
 * 返回随视口变化自动更新的媒体查询结果。
 * 同一查询全局共享一个响应式状态（监听器常驻，数量有限）。
 */
export function useMediaQuery(query: string): Ref<boolean> {
  const cached = cache.get(query)
  if (cached) {
    return cached
  }
  const mql = window.matchMedia(query)
  const value = ref(mql.matches)
  mql.addEventListener('change', (e: MediaQueryListEvent) => {
    value.value = e.matches
  })
  cache.set(query, value)
  return value
}

/** 当前是否处于移动端视口。 */
export function useIsMobile(): Ref<boolean> {
  return useMediaQuery(`(max-width: ${MOBILE_MAX_WIDTH}px)`)
}
