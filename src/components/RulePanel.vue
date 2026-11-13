<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { makeRule, ruleNames } from '../core/types'
import type { Rule, RuleKind } from '../core/types'
import Icon from './Icon.vue'
import { t } from '../core/i18n'

const rules = defineModel<Rule[]>({ required: true })
defineProps<{ disabled: boolean }>()
const activeId = ref('')
const active = computed(() => rules.value.find(rule => rule.id === activeId.value) ?? rules.value[0])
const activeIndex = computed(() => rules.value.findIndex(rule => rule.id === active.value?.id))
const kinds: RuleKind[] = ['replace', 'template', 'affix', 'case']

function add(event: Event) {
  const select = event.target as HTMLSelectElement
  if (!kinds.includes(select.value as RuleKind)) return
  const rule = makeRule(select.value as RuleKind)
  rules.value = [...rules.value, rule]
  activeId.value = rule.id
  select.value = ''
}

function move(offset: number) {
  const index = activeIndex.value
  if (index < 0 || index + offset < 0 || index + offset >= rules.value.length) return
  activeId.value = active.value!.id
  const next = [...rules.value]
  ;[next[index], next[index + offset]] = [next[index + offset]!, next[index]!]
  rules.value = next
}

function remove() {
  const index = activeIndex.value
  const next = rules.value.filter(rule => rule.id !== active.value?.id)
  rules.value = next
  activeId.value = next[Math.min(index, next.length - 1)]?.id ?? ''
}

async function navigate(event: KeyboardEvent, index: number) {
  if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  const next = event.key === 'Home' ? 0 : event.key === 'End' ? rules.value.length - 1 : (index + (event.key === 'ArrowRight' ? 1 : -1) + rules.value.length) % rules.value.length
  activeId.value = rules.value[next]!.id
  await nextTick()
  document.getElementById(`tab-${activeId.value}`)?.focus()
}
</script>

<template>
  <div class="rule-panel">
    <div class="rule-heading"><h2>{{ t('规则') }}</h2><label class="add-rule"><span class="sr-only">{{ t('添加规则') }}</span><select :aria-label="t('添加规则')" :disabled="disabled || rules.length >= 20" @change="add"><option value="">{{ t('添加规则…') }}</option><option v-for="kind in kinds" :key="kind" :value="kind">{{ t(ruleNames[kind]) }}</option></select></label></div>
    <div v-if="rules.length" class="rule-tabs" role="tablist" :aria-label="t('规则顺序')"><button v-for="(rule, index) in rules" :id="`tab-${rule.id}`" :key="rule.id" class="rule-tab" role="tab" :aria-selected="active?.id === rule.id" aria-controls="rule-editor" :tabindex="active?.id === rule.id ? 0 : -1" :disabled="disabled" :class="{ inactive: !rule.enabled }" @click="activeId = rule.id" @keydown="navigate($event, index)"><span class="rule-position">{{ index + 1 }}</span>{{ t(ruleNames[rule.kind]) }}<span v-if="!rule.enabled" class="sr-only">{{ t('，已停用') }}</span></button></div>
    <p v-else class="rules-empty">{{ t('添加一条规则，预览新的名称。') }}</p>
    <div v-if="active" id="rule-editor" role="tabpanel" :aria-labelledby="`tab-${active.id}`">
    <fieldset class="rule-editor" :disabled="disabled">
      <div class="editor-toolbar"><label class="check-label"><input v-model="active.enabled" type="checkbox" />{{ t('启用规则') }}</label><div class="rule-actions"><button class="icon-button" :aria-label="t('上移规则')" :disabled="activeIndex === 0" @click="move(-1)"><Icon name="up" :size="16" /></button><button class="icon-button" :aria-label="t('下移规则')" :disabled="activeIndex === rules.length - 1" @click="move(1)"><Icon name="down" :size="16" /></button><button class="icon-button" :aria-label="t('删除规则')" @click="remove"><Icon name="close" :size="16" /></button></div></div>
      <template v-if="active.kind === 'replace'">
        <label class="field-label">{{ t('查找') }}<input v-model="active.find" :placeholder="t('要查找的文本')" autocomplete="off" spellcheck="false" /></label>
        <div class="replace-options"><label class="check-label"><input v-model="active.regex" type="checkbox" />{{ t('使用正则表达式') }}</label><label class="check-label"><input v-model="active.insensitive" type="checkbox" />{{ t('忽略大小写') }}</label></div>
        <label class="field-label">{{ t('替换为') }}<input v-model="active.value" :placeholder="t('留空则删除匹配内容')" autocomplete="off" spellcheck="false" /></label>
      </template>
      <template v-else-if="active.kind === 'template'">
        <label class="field-label">{{ t('命名格式') }}<input v-model="active.value" :placeholder="t('照片_{n}')" autocomplete="off" spellcheck="false" /></label><p class="field-hint">{{ t('{name} 原名 · {n} 序号 · {date} 修改日期') }}</p>
        <div class="number-fields"><label class="field-label">{{ t('起始序号') }}<input v-model.number="active.start" type="number" min="0" max="999999999" /></label><label class="field-label">{{ t('序号位数') }}<input v-model.number="active.pad" type="number" min="1" max="12" /></label></div>
      </template>
      <template v-else-if="active.kind === 'affix'"><label class="field-label">{{ t('前缀') }}<input v-model="active.prefix" :placeholder="t('添加在名称开头')" autocomplete="off" /></label><label class="field-label">{{ t('后缀') }}<input v-model="active.suffix" :placeholder="t('添加在扩展名前')" autocomplete="off" /></label></template>
      <div v-else-if="active.kind === 'case'" class="case-field"><span class="field-label">{{ t('转换为') }}</span><div class="case-options" role="group" :aria-label="t('大小写转换')"><button :aria-pressed="active.mode === 'lower'" :class="{ chosen: active.mode === 'lower' }" @click="active.mode = 'lower'">{{ t('aa 小写') }}</button><button :aria-pressed="active.mode === 'upper'" :class="{ chosen: active.mode === 'upper' }" @click="active.mode = 'upper'">{{ t('AA 大写') }}</button></div></div>
    </fieldset>
    </div>
  </div>
</template>
