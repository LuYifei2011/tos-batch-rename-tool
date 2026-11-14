<script setup lang="ts">
import renameIcon from '../assets/rename.svg'
import { ArrowDownToLine, ArrowRight, Check, ChevronDown, ChevronUp, CircleHelp, File, Folder, History, Plus, RefreshCw, Search, Settings, TriangleAlert, Undo2, X } from '@lucide/vue'
import type { Component } from 'vue'
defineProps<{ name: string; size?: number }>()
const icons: Record<string, Component> = {
 settings: Settings, undo: Undo2, warning: TriangleAlert, folder: Folder, file: File,
 history: History, up: ChevronUp, down: ChevronDown, refresh: RefreshCw,
 plus: Plus, close: X, search: Search, arrow: ArrowRight, download: ArrowDownToLine, check: Check, help: CircleHelp,
}
</script>
<template><img v-if="name === 'rename'" :src="renameIcon" :width="size ?? 20" :height="size ?? 20" alt="" aria-hidden="true" /><component :is="icons[name] ?? File" v-else :size="size ?? 20" :stroke-width="1.6" aria-hidden="true" /></template>
