<script setup lang="ts">
// 侧栏导航：品牌区 + 菜单；桌面端支持折叠（仅图标），移动端抽屉中恒为展开。
import { computed, h, type Component } from 'vue'
import { useRoute } from 'vue-router'
import { NIcon, NMenu, type MenuOption } from 'naive-ui'
import {
  AddCircleOutline,
  AppsOutline,
  CodeOutline,
  DocumentTextOutline,
  HomeOutline,
  LayersOutline,
  PulseOutline,
  RocketOutline,
  SettingsOutline,
  ShieldCheckmarkOutline,
} from '@vicons/ionicons5'

const props = defineProps<{
  /** 桌面端折叠状态（抽屉中恒为展开）。 */
  collapsed?: boolean
}>()

defineEmits<{ select: [key: string] }>()

const route = useRoute()

/** 菜单图标统一包一层 n-icon。 */
function renderIcon(icon: Component) {
  return () => h(NIcon, null, { default: () => h(icon) })
}

const menuOptions: MenuOption[] = [
  { label: '概览', key: '/dashboard', icon: renderIcon(HomeOutline) },
  { label: '小程序管理', key: '/authorizers', icon: renderIcon(AppsOutline) },
  { label: '代码模板', key: '/templates', icon: renderIcon(CodeOutline) },
  { label: '批量任务', key: '/jobs', icon: renderIcon(LayersOutline) },
  { label: '新建任务', key: '/jobs/new', icon: renderIcon(AddCircleOutline) },
  { label: '审核管理', key: '/audits', icon: renderIcon(ShieldCheckmarkOutline) },
  { label: '发布管理', key: '/releases', icon: renderIcon(RocketOutline) },
  { label: '前置体检', key: '/preflight', icon: renderIcon(PulseOutline) },
  { label: '日志', key: '/logs', icon: renderIcon(DocumentTextOutline) },
  { label: '设置', key: '/settings', icon: renderIcon(SettingsOutline) },
]

/** 参与前缀匹配的菜单 key（新建任务为独立入口，不参与详情页归属匹配）。 */
const prefixKeys: string[] = menuOptions
  .map((item) => String(item.key))
  .filter((key) => key !== '/jobs/new')

/**
 * 当前菜单选中项：
 * 1. 精确命中菜单路径直接高亮；
 * 2. 详情页（/jobs/:id、/authorizers/:appid 等）把高亮归给所属的列表菜单。
 */
const activeKey = computed<string>(() => {
  const path = route.path
  const exact = prefixKeys.find((key) => key === path)
  if (exact) {
    return exact
  }
  // 取最长匹配前缀，避免 '/jobs' 抢先命中 '/jobs/new'。
  const matched = prefixKeys
    .filter((key) => path.startsWith(`${key}/`))
    .sort((a, b) => b.length - a.length)
  if (matched.length > 0) {
    return matched[0] ?? path
  }
  return path
})

/** 折叠仅作用于桌面端侧栏（抽屉内 collapsed 传 false）。 */
const isCollapsed = computed<boolean>(() => props.collapsed === true)
</script>

<template>
  <div class="sider-content">
    <div class="brand" :class="{ 'brand--collapsed': isCollapsed }">
      <div class="brand-logo">
        <n-icon :size="22" color="#1668dc">
          <apps-outline />
        </n-icon>
      </div>
      <div v-show="!isCollapsed" class="brand-text">
        <div class="brand-title">微信第三方平台管理</div>
      </div>
    </div>
    <n-menu
      :value="activeKey"
      :options="menuOptions"
      :root-indent="16"
      :indent="24"
      :collapsed="isCollapsed"
      :collapsed-width="64"
      :collapsed-icon-size="22"
      @update:value="(key: string) => $emit('select', key)"
    />
  </div>
</template>

<style scoped>
.sider-content {
  height: 100%;
}

.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 16px 14px;
}

.brand--collapsed {
  justify-content: center;
  padding: 16px 8px;
}

.brand-logo {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: #d9e8fb;
  flex-shrink: 0;
}

.brand-text {
  min-width: 0;
}

.brand-title {
  font-size: 15px;
  font-weight: 600;
  color: #17233d;
  white-space: nowrap;
}
</style>
