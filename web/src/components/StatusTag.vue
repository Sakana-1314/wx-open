<script setup lang="ts">
// 通用状态标签：把后端状态枚举映射为中文文案 + 语义色。
// 语义色只用于状态标签，不用于普通文字（遵循项目 UI 规范）。
import { computed } from 'vue'
import { NTag } from 'naive-ui'

import {
  auditStatusText,
  auditStatusType,
  authorizationStatusText,
  booleanStatusType,
  itemStatusText,
  itemStatusType,
  jobStatusText,
  jobStatusType,
  type StatusKind,
  type TagType,
} from '@/utils/format'

const props = defineProps<{
  /** 状态原始值：作业/条目/授权为字符串枚举，审核为数字，布尔型为 true/false。 */
  status: string | number | boolean
  /** 状态语义域，决定映射规则；默认按作业状态处理。 */
  kind?: StatusKind
  /** 自定义文案，优先级高于 kind 映射（颜色仍由 status 决定）。 */
  label?: string
}>()

/** 按语义域把状态映射为 { 文案, 颜色 }。 */
const config = computed<{ type: TagType; label: string }>(() => {
  switch (props.kind) {
    case 'item':
      return { type: itemStatusType(String(props.status)), label: itemStatusText(String(props.status)) }
    case 'audit':
      return { type: auditStatusType(Number(props.status)), label: auditStatusText(Number(props.status)) }
    case 'boolean':
      return {
        type: booleanStatusType(props.status === true),
        label: props.status === true ? '正常' : '异常',
      }
    case 'authorizer':
      return {
        type: props.status === 'authorized' ? 'success' : 'error',
        label: authorizationStatusText(String(props.status)),
      }
    default:
      return { type: jobStatusType(String(props.status)), label: jobStatusText(String(props.status)) }
  }
})
</script>

<template>
  <n-tag :type="config.type" size="small" round :bordered="false">
    {{ label ?? config.label }}
  </n-tag>
</template>
