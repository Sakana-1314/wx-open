<script setup lang="ts">
// 页面统一标题块：标题 + 一行说明（正文色，不使用灰色小字）+ 右侧操作插槽。
// 用法：
//   <div class="page">
//     <page-header title="小程序管理" description="授权方清单、筛选与同步">
//       <template #extra><n-button type="primary">同步</n-button></template>
//     </page-header>
//     <n-card>…</n-card>
//   </div>
defineProps<{
  /** 页面标题（与路由 meta.title 保持一致）。 */
  title: string
  /** 一行说明，说明本页用途；留空则不渲染。 */
  description?: string
}>()
</script>

<template>
  <div class="page-header">
    <div class="page-header-main">
      <h1 class="page-header-title">{{ title }}</h1>
      <p v-if="description" class="page-header-desc">{{ description }}</p>
    </div>
    <div v-if="$slots.extra" class="page-header-extra">
      <slot name="extra" />
    </div>
  </div>
</template>

<style scoped>
.page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 14px;
}

.page-header-main {
  min-width: 0;
}

.page-header-title {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  line-height: 1.4;
  color: #17233d;
}

/* 说明使用正文色 14px，避免灰色小字。 */
.page-header-desc {
  margin: 4px 0 0;
  font-size: 14px;
  line-height: 1.5;
  color: #3d4a5c;
}

.page-header-extra {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

@media (max-width: 820px) {
  /* 窄屏下操作区占满整行，避免按钮被挤压。 */
  .page-header-extra {
    width: 100%;
  }
}
</style>
