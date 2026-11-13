<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import type { Entry, Preview } from '../core/types'
import Icon from './Icon.vue'
import { localizeMessage, number, t } from '../core/i18n'

const props = defineProps<{ entries: Entry[]; rows: Preview[]; selected: Set<string>; disabled: boolean; loading: boolean; computing: boolean; filter: string; includeFolders: boolean }>()
const emit = defineEmits<{ toggle: [name: string]; open: [name: string]; selectAll: [names: string[], checked: boolean] }>()
const area = ref<HTMLElement>()
const scroll = ref(0)
const height = ref(500)
const rowHeight = ref(34)
const previewMap = computed(() => new Map(props.rows.map(row => [row.name, row])))
const filtered = computed(() => props.entries.map(entry => ({ entry, row: previewMap.value.get(entry.name) })).filter(({ row }) => props.filter === 'all' || (props.filter === 'changed' ? row?.status === 'ready' : row?.status === 'invalid' || row?.status === 'conflict')))
const start = computed(() => Math.max(0, Math.min(filtered.value.length - 1, Math.floor(scroll.value / rowHeight.value) - 5)))
const visible = computed(() => filtered.value.slice(start.value, start.value + Math.ceil(height.value / rowHeight.value) + 10))
const selectable = computed(() => filtered.value.filter(({ entry }) => entry.kind !== 'symlink' && (props.includeFolders || entry.kind !== 'dir')).map(({ entry }) => entry.name))
const allChecked = computed(() => selectable.value.length > 0 && selectable.value.every(name => props.selected.has(name)))
const someChecked = computed(() => !allChecked.value && selectable.value.some(name => props.selected.has(name)))
const changed = computed(() => props.rows.filter(row => row.status === 'ready').length)
const hasProblem = (row?: Preview) => row?.status === 'invalid' || row?.status === 'conflict'
let observer: ResizeObserver
watch(() => [props.filter, props.entries], () => { scroll.value = 0; if (area.value) area.value.scrollTop = 0 })
onMounted(() => { observer = new ResizeObserver(records => { height.value = records[0]?.contentRect.height ?? 500; if (area.value) rowHeight.value = parseFloat(getComputedStyle(area.value).getPropertyValue('--file-row-height')) || 34 }); if (area.value) observer.observe(area.value) })
onUnmounted(() => observer?.disconnect())
</script>

<template>
  <div class="file-table" role="table" :aria-label="t('重命名预览')" :aria-rowcount="filtered.length + 1" aria-colcount="3" :aria-busy="loading || computing">
    <div class="file-row table-head" role="row"><span role="columnheader" class="select-cell"><label class="selection-target"><span class="sr-only">{{ t('全选当前列表') }}</span><input type="checkbox" :aria-label="t('全选当前列表')" :checked="allChecked" :indeterminate="someChecked" :disabled="disabled || !selectable.length" @change="emit('selectAll', selectable, !allChecked)" /></label></span><span role="columnheader">{{ t('原名称') }} <span class="column-count">({{ number(selected.size) }})</span></span><div role="columnheader" class="new-name-heading"><span>{{ t('新名称') }} <span class="column-count">({{ number(changed) }})</span></span><slot name="filter" /></div></div>
    <div ref="area" class="table-scroll" @scroll="scroll = ($event.target as HTMLElement).scrollTop">
      <div v-if="loading" class="table-empty" role="status">{{ t('正在读取目录…') }}</div>
      <div v-else-if="!filtered.length" class="table-empty"><Icon name="folder" :size="30" /><strong>{{ t(filter !== 'all' ? (computing ? '正在更新预览…' : '没有匹配的项目') : '此目录为空') }}</strong><span v-if="filter !== 'all'">{{ t('可切换为“全部文件”查看其他项目。') }}</span><span v-else>{{ t('可返回“共享文件夹”选择其他目录。') }}</span></div>
      <div v-else class="virtual-space" :style="{ height: `${filtered.length * rowHeight}px` }"><div class="virtual-rows" :style="{ transform: `translateY(${start * rowHeight}px)` }">
        <div v-for="({ entry, row }, index) in visible" :key="entry.name" class="file-row data-row" role="row" :aria-rowindex="start + index + 2" :class="{ selected: selected.has(entry.name), problem: hasProblem(row), excluded: !selected.has(entry.name) }">
          <span role="cell" class="select-cell"><label class="selection-target"><span class="sr-only">{{ t('选择 {name}', { name: entry.name }) }}</span><input type="checkbox" :aria-label="t('选择 {name}', { name: entry.name })" :checked="selected.has(entry.name)" :disabled="disabled || entry.kind === 'symlink' || (!includeFolders && entry.kind === 'dir')" @change="emit('toggle', entry.name)" /></label></span>
          <div role="cell" class="original-name"><Icon :name="entry.kind === 'dir' ? 'folder' : 'file'" :size="17" :class="{ 'folder-icon': entry.kind === 'dir' }" /><button v-if="entry.kind === 'dir'" class="name-button" :disabled="disabled" :title="t('打开 {name}', { name: entry.name })" @click="emit('open', entry.name)">{{ entry.name }}</button><span v-else :title="entry.name">{{ entry.name }}</span></div>
          <div role="cell" class="new-name" :class="{ changed: row?.status === 'ready' }" :title="hasProblem(row) ? `${row?.newName}: ${localizeMessage(row?.reason ?? '')}` : row?.newName ?? entry.name"><span class="filename">{{ row?.newName ?? entry.name }}</span><span v-if="hasProblem(row)" class="row-error"><Icon name="warning" :size="12" />{{ localizeMessage(row?.reason ?? '') }}</span><span v-else-if="entry.kind === 'symlink'" class="row-note">{{ t('跳过符号链接') }}</span></div>
        </div>
      </div></div>
    </div>
  </div>
</template>
