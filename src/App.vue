<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, shallowRef, watch } from 'vue'
import FileTable from './components/FileTable.vue'
import Icon from './components/Icon.vue'
import RulePanel from './components/RulePanel.vue'
import RootList from './components/RootList.vue'
import { TosAppShell, TosConnectionReport as ConnectionReport, TosPreferencesForm as PreferencesForm } from 'tos-app-ui-kit'
import { api, backendVersion, connectionDiagnostic, denied } from './core/api'
import { APP_VERSION } from './core/build'
import { localizeMessage, number, t } from './core/i18n'
import { makeRule } from './core/types'
import type { Entry, Job, Preview, Root } from './core/types'

const roots = ref<Root[]>([])
const mode = ref('')
const root = ref('')
const path = ref('')
const entries = shallowRef<Entry[]>([])
const rows = shallowRef<Preview[]>([])
const selected = shallowRef(new Set<string>())
const rules = ref([makeRule('replace')])
const includeFolders = ref(false)
const filter = ref('all')
const loading = ref(false)
const initializing = ref(true)
const computing = ref(false)
const checking = ref(false)
const running = ref(false)
const activeJobId = ref('')
const error = ref('')
const job = ref<Job>()
const dialog = ref<HTMLDialogElement>()
const dialogKind = ref<'confirm' | 'help' | 'result' | 'settings'>('confirm')
const confirmPlan = ref('')
const confirmCount = ref(0)
const undoing = ref(false)
const resultItems = shallowRef<Job['items']>([])
const activeRoot = computed(() => roots.value.find(item => item.id === root.value))
const atRoots = computed(() => !root.value)
const crumbs = computed(() => path.value.split('/').filter(Boolean))
const ready = computed(() => rows.value.filter(row => row.status === 'ready'))
const problems = computed(() => rows.value.filter(row => row.status === 'conflict' || row.status === 'invalid').length)
const busy = computed(() => loading.value || checking.value || running.value)
const currentJob = computed(() => !!job.value && job.value.root === root.value && job.value.path === path.value)
const canExecute = computed(() => !!root.value && !busy.value && !computing.value && !error.value && !problems.value && ready.value.length > 0 && selected.value.size <= 50000)
const showUndo = computed(() => currentJob.value && !!job.value && job.value.done > 0 && !job.value.undoOf && !['running', 'interrupted'].includes(job.value.state))
const canUndo = computed(() => !busy.value && showUndo.value)
const jobStates: Record<string, string> = { running: '正在重命名', done: '已完成', cancelled: '已停止', partial: '部分失败', interrupted: '需要核对文件状态' }
let worker: Worker | undefined
let debounce: ReturnType<typeof setTimeout>
let watchdog: ReturnType<typeof setTimeout>
let poll: ReturnType<typeof setTimeout>
let loadGeneration = 0
let disposed = false
const message = (value: unknown) => value instanceof Error ? value.message : String(value)
const reopen = () => location.reload()

async function load(nextRoot = root.value, nextPath = path.value) {
  if (!nextRoot) { showRoots(); return }
  const generation = ++loadGeneration
  loading.value = true
  error.value = ''
  filter.value = 'all'
  try {
    const data = await api<{ entries: Entry[] }>(`/list?root=${encodeURIComponent(nextRoot)}&path=${encodeURIComponent(nextPath)}`)
    if (generation !== loadGeneration || disposed) return
    root.value = nextRoot
    path.value = nextPath
    entries.value = data.entries
    rows.value = []
    selected.value = new Set(data.entries.filter(entry => entry.kind === 'file' || (includeFolders.value && entry.kind === 'dir')).map(entry => entry.name))
  } catch (value) {
    if (generation === loadGeneration) error.value = message(value)
  } finally {
    if (generation === loadGeneration) loading.value = false
  }
}

function showRoots() {
  ++loadGeneration
  root.value = ''
  path.value = ''
  entries.value = []
  rows.value = []
  selected.value = new Set()
  filter.value = 'all'
  error.value = ''
}

function up() {
  if (path.value) void load(root.value, crumbs.value.slice(0, -1).join('/'))
  else showRoots()
}

async function refresh() {
  loading.value = true
  error.value = ''
  try {
    const info = await api<{ mode: string; roots: Root[] }>('/info')
    if (disposed) return false
    mode.value = info.mode
    roots.value = info.roots
    const current = info.roots.find(item => item.id === root.value)
    if (current) await load(current.id, path.value)
    else showRoots()
    return !error.value
  } catch (value) { error.value = message(value); return false }
  finally { loading.value = false }
}

function toggle(name: string) {
  const next = new Set(selected.value)
  if (next.has(name)) next.delete(name)
  else next.add(name)
  selected.value = next
}

function selectAll(names: string[], checked: boolean) {
  const next = new Set(selected.value)
  for (const name of names) {
    if (checked) next.add(name)
    else next.delete(name)
  }
  selected.value = next
}

function compute() {
  worker?.terminate()
  worker = undefined
  clearTimeout(debounce)
  clearTimeout(watchdog)
  if (!root.value) { rows.value = []; computing.value = false; return }
  computing.value = true
  debounce = setTimeout(() => {
    const current = new Worker(new URL('./core/preview.worker.ts', import.meta.url), { type: 'module' })
    worker = current
    current.onmessage = event => {
      if (worker !== current) return
      clearTimeout(watchdog)
      computing.value = false
      rows.value = event.data.rows ?? []
      if (event.data.error) error.value = event.data.error
      current.terminate()
      worker = undefined
    }
    current.onerror = () => {
      if (worker !== current) return
      clearTimeout(watchdog)
      computing.value = false
      rows.value = []
      error.value = '无法计算预览，请检查规则后重试。'
      current.terminate()
      worker = undefined
    }
    current.postMessage({ entries: entries.value, selected: [...selected.value], rules: JSON.parse(JSON.stringify(rules.value)) })
    watchdog = setTimeout(() => {
      if (worker !== current) return
      current.terminate()
      worker = undefined
      computing.value = false
      rows.value = []
      error.value = '预览计算超时，请简化正则表达式或规则。'
    }, 3000)
  }, 140)
}

watch([entries, selected, root], compute)
watch(rules, () => { error.value = ''; compute() }, { deep: true })
watch(includeFolders, () => {
  selected.value = new Set(entries.value.filter(entry => entry.kind === 'file' || (includeFolders.value && entry.kind === 'dir')).map(entry => entry.name))
})

async function showDialog(kind: typeof dialogKind.value) {
  dialogKind.value = kind
  await nextTick()
  dialog.value?.showModal()
}

async function validate() {
  checking.value = true
  error.value = ''
  undoing.value = false
  try {
    const plan = await api<{ id: string; count: number }>('/plans', {
      root: root.value, path: path.value,
      items: ready.value.map(row => ({ from: row.name, to: row.newName, fingerprint: entries.value[row.index]!.fingerprint, state: 'pending' })),
    })
    confirmPlan.value = plan.id
    confirmCount.value = plan.count
    await showDialog('confirm')
  } catch (value) { error.value = message(value) }
  finally { checking.value = false }
}

async function execute() {
  checking.value = true
  try {
    const result = await api<{ id: string }>('/execute', { plan: confirmPlan.value })
    dialog.value?.close()
    activeJobId.value = result.id
    job.value = undefined
    running.value = true
    await pollJob(result.id)
  } catch (value) {
    error.value = message(value)
    dialog.value?.close()
  } finally { checking.value = false }
}

async function pollJob(id: string) {
  if (disposed || denied.value) return
  try {
    job.value = await api<Job>(`/jobs/${id}?summary=1`)
    if (disposed) return
    running.value = job.value.state === 'running'
    if (running.value) poll = setTimeout(() => pollJob(id), 350)
    else await load(job.value.root, job.value.path)
  } catch (value) {
    error.value = `无法获取执行状态：${message(value)}`
    if (!denied.value) poll = setTimeout(() => pollJob(id), 2000)
  }
}

async function undo() {
  if (!job.value) return
  checking.value = true
  error.value = ''
  try {
    const plan = await api<{ id: string; count: number }>('/undo', { id: job.value.id })
    confirmPlan.value = plan.id
    confirmCount.value = plan.count
    undoing.value = true
    await showDialog('confirm')
  } catch (value) { error.value = message(value) }
  finally { checking.value = false }
}

async function stop() {
  const id = activeJobId.value || (job.value?.state === 'running' ? job.value.id : '')
  if (!id) return
  try { await api('/cancel', { id }) }
  catch (value) { error.value = message(value) }
}

async function showResult() {
  if (!job.value) return
  try {
    const result = await api<Job>(`/jobs/${job.value.id}`)
    resultItems.value = result.items.filter(item => item.state !== 'done').slice(0, 100)
    await showDialog('result')
  } catch (value) { error.value = message(value) }
}

onMounted(async () => {
  try {
    if (!await refresh()) return
    const jobs = await api<Job[]>('/jobs')
    job.value = jobs.find(item => item.state === 'running') ?? jobs[0]
    if (job.value?.state === 'running') { running.value = true; activeJobId.value = job.value.id; await pollJob(job.value.id) }
  } catch (value) { error.value = message(value) }
  finally { initializing.value = false }
})

onUnmounted(() => {
  disposed = true
  worker?.terminate()
  clearTimeout(debounce)
  clearTimeout(watchdog)
  clearTimeout(poll)
})
</script>

<template>
  <main v-if="denied" class="access-denied">
    <h1>{{ localizeMessage(denied) }}</h1>
    <PreferencesForm />
    <ConnectionReport v-if="connectionDiagnostic" :report="connectionDiagnostic" />
    <button class="button primary" @click="reopen">{{ t('重新加载') }}</button>
  </main>
  <TosAppShell v-else :title="t('批量重命名')" scroll="hidden" content-class="app-content">
    <template #icon><Icon class="app-brand-icon" name="rename" :size="21" /></template>
    <template v-if="mode === 'demo'" #header-extra><span class="demo-label">{{ t('演示目录') }}</span></template>
    <div v-if="initializing" class="startup-loading" role="status">{{ t('正在连接 TOS…') }}</div>
    <div v-else class="workspace">
      <section class="controls-pane" :aria-label="t('规则与操作')">
        <RulePanel v-model="rules" :disabled="busy" />
        <footer class="apply-area">
          <div class="apply-options"><label class="check-label"><input v-model="includeFolders" type="checkbox" :disabled="busy || atRoots" />{{ t('包含文件夹') }}</label><div class="utility-actions"><button class="icon-button" :aria-label="t('语言与主题')" :title="t('语言与主题')" @click="showDialog('settings')"><Icon name="settings" :size="17" /></button><button class="icon-button" :aria-label="t('使用说明')" :title="t('使用说明')" @click="showDialog('help')"><Icon name="help" :size="17" /></button></div></div>
          <p class="apply-hint">{{ t('规则按顺序应用，保留扩展名。') }}</p>
          <p v-if="selected.size > 50000" class="problem-text" role="alert">{{ t('单批最多选择 50,000 项。') }}</p>
          <button v-if="problems" class="problem-link" @click="filter = 'problems'">{{ t('{count} 项名称需要处理', { count: number(problems) }) }}</button>
          <div class="apply-buttons"><button v-if="showUndo" class="button secondary" :disabled="!canUndo" :title="t('撤销最近一次任务的成功项')" @click="undo"><Icon name="undo" :size="16" />{{ t('撤销') }}</button><button v-if="running" class="button primary" @click="stop">{{ t('停止') }}</button><button v-else class="button primary" :disabled="!canExecute" @click="validate">{{ checking ? t('正在检查…') : ready.length ? t('重命名 {count} 项', { count: number(ready.length) }) : t('重命名') }}</button></div>
        </footer>
      </section>
      <section class="preview-pane" :aria-label="t('文件预览')">
        <nav class="path-bar" :aria-label="t('当前目录')"><button class="icon-button" :aria-label="t('上一级目录')" :disabled="busy || atRoots" @click="up"><Icon name="up" :size="17" /></button><div class="breadcrumbs"><button :disabled="busy" :aria-current="atRoots ? 'page' : undefined" @click="showRoots">{{ t('共享文件夹') }}</button><template v-if="activeRoot"><span>/</span><button :disabled="busy" @click="load(root, '')">{{ activeRoot.name }}</button></template><template v-for="(crumb, index) in crumbs" :key="index"><span>/</span><button :disabled="busy" @click="load(root, crumbs.slice(0, index + 1).join('/'))">{{ crumb }}</button></template></div><button class="icon-button" :aria-label="t('刷新目录')" :disabled="busy" @click="refresh"><Icon name="refresh" :size="17" /></button></nav>
        <div v-if="error" class="message error-message" role="alert"><span>{{ localizeMessage(error) }}</span><button class="icon-button" :aria-label="t('关闭错误提示')" @click="error = ''"><Icon name="close" :size="16" /></button></div>
        <ConnectionReport v-if="error && connectionDiagnostic" class="inline-diagnostic" :report="connectionDiagnostic" />
        <RootList v-if="atRoots" :roots="roots" :loading="loading" :disabled="busy" @open="load($event, '')" />
        <FileTable v-else :entries="entries" :rows="rows" :selected="selected" :disabled="busy" :loading="loading" :computing="computing" :filter="filter" :include-folders="includeFolders" @toggle="toggle" @select-all="selectAll" @open="load(root, [...crumbs, $event].join('/'))">
          <template #filter><label class="preview-filter"><span class="sr-only">{{ t('显示文件') }}</span><select v-model="filter" :aria-label="t('显示文件')"><option value="all">{{ t('全部文件') }}</option><option value="changed">{{ t('待重命名') }}</option><option value="problems">{{ t('冲突与无效') }}</option></select></label></template>
        </FileTable>
        <footer class="preview-footer" aria-live="polite">
          <span v-if="loading">{{ t('正在读取目录…') }}</span><span v-else-if="computing">{{ t('正在更新预览…') }}</span><span v-else-if="running">{{ t('正在重命名') }}<span v-if="job"> · {{ number(job.done + job.failed) }} / {{ number(job.total) }}</span>…</span><span v-else-if="atRoots">{{ t('{count} 个共享文件夹', { count: number(roots.length) }) }}</span>
          <span v-else-if="job && currentJob">{{ t(job.undoOf ? '撤销状态：{state}' : '任务状态：{state}', { state: t(jobStates[job.state] ?? job.state) }) }} · {{ t('成功 {count} 项', { count: number(job.done) }) }}<span v-if="job.failed"> · {{ t('失败 {count} 项', { count: number(job.failed) }) }}</span><span v-if="job.state === 'cancelled'"> · {{ t('未执行 {count} 项', { count: number(job.total - job.done - job.failed) }) }}</span></span>
          <span v-else>{{ t('{count} 个项目 · 已选 {selected} 项', { count: number(entries.length), selected: number(selected.size) }) }}</span>
          <button v-if="job && currentJob && ['partial', 'interrupted', 'cancelled'].includes(job.state) && !busy" class="text-button" @click="showResult">{{ t('查看结果') }}</button>
        </footer>
      </section>
    </div>
    <dialog ref="dialog" class="app-dialog" aria-labelledby="dialog-title" @click="($event.target === dialog) && !checking && dialog?.close()" @cancel="checking && $event.preventDefault()">
      <template v-if="dialogKind === 'confirm'">
        <header class="dialog-heading"><h2 id="dialog-title">{{ t(undoing ? '撤销重命名' : '确认重命名') }}</h2><button class="icon-button" :aria-label="t('关闭确认')" :disabled="checking" @click="dialog?.close()"><Icon name="close" :size="18" /></button></header>
        <p>{{ t(undoing ? '即将恢复 {count} 项的名称。' : '即将修改 {count} 项的名称。', { count: number(confirmCount) }) }}</p><p v-if="mode === 'tos'" class="dialog-note">{{ t('执行前会再次检查源文件和名称冲突。处理期间请勿使用其他应用修改此目录。') }}</p><p v-else class="dialog-note">{{ t('源文件会再次校验，已有目标不会被覆盖。') }}</p>
        <footer class="dialog-actions"><button class="button secondary" :disabled="checking" @click="dialog?.close()">{{ t('取消') }}</button><button class="button primary" :disabled="checking" @click="execute">{{ t(checking ? '正在提交…' : undoing ? '确认撤销' : '确认重命名') }}</button></footer>
      </template>
      <template v-else-if="dialogKind === 'help'">
        <header class="dialog-heading"><h2 id="dialog-title">{{ t('使用说明') }}</h2><button class="icon-button" :aria-label="t('关闭说明')" @click="dialog?.close()"><Icon name="close" :size="18" /></button></header>
        <ol><li>{{ t('在右侧打开共享文件夹，再进入要处理的目录。') }}</li><li>{{ t('勾选文件，在左侧编辑或添加规则。') }}</li><li>{{ t('检查右侧新名称，确认后重命名。') }}</li></ol>
        <p>{{ t('规则按排列顺序应用。序号按所选文件的名称排序生成；筛选只改变显示范围。') }}</p><p class="dialog-note">{{ t('正则替换支持 $1、$2 捕获组。扩展名保留，符号链接跳过，大小写改名与已有目标会提示冲突。') }}</p><p v-if="mode === 'demo'" class="dialog-note">{{ t('当前操作对象为隔离目录中的测试文件。') }}</p>
        <p class="dialog-note">{{ t('前端 {client} · 后端 {server}', { client: APP_VERSION, server: backendVersion || t('未返回版本') }) }}</p>
      </template>
      <template v-else-if="dialogKind === 'settings'">
        <header class="dialog-heading"><h2 id="dialog-title">{{ t('语言与主题') }}</h2><button class="icon-button" :aria-label="t('关闭设置')" @click="dialog?.close()"><Icon name="close" :size="18" /></button></header>
        <PreferencesForm />
      </template>
      <template v-else>
        <header class="dialog-heading"><h2 id="dialog-title">{{ t('任务结果') }}</h2><button class="icon-button" :aria-label="t('关闭结果')" @click="dialog?.close()"><Icon name="close" :size="18" /></button></header>
        <p v-if="job">{{ t('成功 {done} 项，失败 {failed} 项，未完成 {pending} 项。', { done: number(job.done), failed: number(job.failed), pending: number(job.total - job.done - job.failed) }) }}</p><p class="dialog-note">{{ t('以下最多显示 100 个未完成项目。') }}</p>
        <div class="result-list"><div v-for="item in resultItems" :key="item.from"><strong>{{ item.from }}</strong><p>{{ localizeMessage(item.error || (item.state === 'pending' ? '未执行' : '请核对文件实际状态')) }}</p></div></div>
      </template>
    </dialog>
  </TosAppShell>
</template>
