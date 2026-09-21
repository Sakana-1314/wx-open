<script setup lang="ts">
import { dateZhCN, zhCN, NConfigProvider, NDialogProvider, NMessageProvider } from 'naive-ui'

import { themeOverrides } from '@/theme/theme'
</script>

<template>
  <n-config-provider :theme-overrides="themeOverrides" :locale="zhCN" :date-locale="dateZhCN">
    <n-message-provider>
      <n-dialog-provider>
        <router-view />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>