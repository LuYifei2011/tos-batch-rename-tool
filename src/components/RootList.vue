<script setup lang="ts">
import type { Root } from '../core/types'
import Icon from './Icon.vue'
import { t } from '../core/i18n'

defineProps<{ roots: Root[]; loading: boolean; disabled: boolean }>()
defineEmits<{ open: [id: string] }>()
</script>

<template>
  <div class="root-list file-table" role="table" :aria-label="t('共享文件夹')" :aria-rowcount="roots.length + 1" aria-colcount="3" :aria-busy="loading">
    <div class="file-row table-head root-row" role="row"><span role="columnheader"><span class="sr-only">{{ t('文件夹图标') }}</span></span><span role="columnheader">{{ t('名称') }}</span><span role="columnheader">{{ t('类型') }}</span></div>
    <div class="table-scroll">
      <div v-if="loading" class="table-empty" role="status">{{ t('正在读取共享文件夹…') }}</div>
      <div v-else-if="!roots.length" class="table-empty"><Icon name="folder" :size="30" /><strong>{{ t('暂无可访问的共享文件夹') }}</strong><span>{{ t('点击刷新，重新读取已授权目录。') }}</span></div>
      <div v-for="item in loading ? [] : roots" :key="item.id" class="file-row data-row root-row" role="row">
        <span role="cell" class="select-cell"><Icon name="folder" :size="18" class="folder-icon" /></span>
        <div role="cell" class="original-name"><button class="name-button" :disabled="disabled" :title="t('打开 {name}', { name: item.name })" @click="$emit('open', item.id)">{{ item.name }}</button></div>
        <span role="cell" class="root-kind">{{ t('共享目录') }}</span>
      </div>
    </div>
  </div>
</template>
