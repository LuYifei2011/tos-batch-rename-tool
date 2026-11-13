<script setup lang="ts">
import renameIcon from '../assets/rename.svg'
defineProps<{ name: string; size?: number }>()
const paths: Record<string,string> = {
 settings: 'M4 6h16 M4 12h16 M4 18h16 M8 4v4 M16 10v4 M10 16v4',
 undo: 'M8 5 3 10l5 5 M3 10h10a7 7 0 0 1 7 7v3', warning: 'M12 3 2 21h20L12 3 M12 9v5 M12 17v1',
 folder: 'M3 6h7l2 2h9v12H3z M3 6V4h7l2 2', file: 'M6 3h8l4 4v14H6z M14 3v5h4 M9 12h6 M9 16h6',
 history: 'M4 5v5h5 M4 10a8 8 0 1 1 0 5 M12 7v5l3 2', up: 'm6 14 6-6 6 6', down: 'm6 10 6 6 6-6',
 refresh: 'M20 5v5h-5 M4 19v-5h5 M5 9a7 7 0 0 1 11-4l4 5 M4 14l4 5a7 7 0 0 0 11-4',
 plus: 'M12 5v14 M5 12h14', close: 'm6 6 12 12 M18 6 6 18', search: 'M10 17a7 7 0 1 0 0-14 7 7 0 0 0 0 14 M15 15l6 6',
 arrow: 'M4 12h16 M15 7l5 5-5 5', download: 'M12 3v12 m-5-5 5 5 5-5 M4 17v4h16v-4', check: 'm5 12 4 4L19 6', help:'M9 8a3 3 0 1 1 5 2c-2 1-2 2-2 4 M12 17v1 M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20',
}
</script>
<template><img v-if="name === 'rename'" :src="renameIcon" :width="size ?? 20" :height="size ?? 20" alt="" aria-hidden="true" /><svg v-else :width="size ?? 20" :height="size ?? 20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round" stroke-linecap="round" aria-hidden="true"><path :d="paths[name] ?? paths.file" /></svg></template>
