<script setup lang="ts">
import { computed, ref } from 'vue'
import type { ConnectionDiagnostic } from '../core/api'
import { t } from '../core/i18n'

const props = defineProps<{ report: ConnectionDiagnostic }>()
const field = ref<HTMLTextAreaElement>()
const text = computed(() => JSON.stringify(props.report, null, 2))
function selectReport() { field.value?.focus(); field.value?.select() }
</script>

<template>
  <section class="connection-report" :aria-label="t('连接诊断')">
    <h2>{{ t('连接诊断') }}</h2>
    <dl>
      <dt>{{ t('前端 / 后端版本') }}</dt><dd>{{ report.clientVersion }} / {{ report.serverVersion || t('响应未提供') }}</dd>
      <dt>{{ t('响应') }}</dt><dd>{{ report.status ?? t('网络失败') }} · {{ report.responseFormat }} · {{ report.responseBytes }} B<span v-if="report.responseCode"> · {{ report.responseCode }}</span></dd>
      <dt>CSRF Cookie</dt><dd>{{ t(report.csrfCookieAvailable ? '页面可读取' : '页面未读取到') }}</dd>
      <dt>{{ t('页面设置 CSRF 请求头') }}</dt><dd>{{ t(report.csrfHeaderConfigured ? '是' : '否') }}</dd>
    </dl>
    <p class="dialog-note">{{ t('报告仅记录凭据是否可读，不包含 Cookie 或令牌值。页面读取结果不能代表系统最终发送的全部凭据。') }}</p>
    <details>
      <summary>{{ t('查看完整报告') }}</summary>
      <label class="sr-only" for="connection-report-text">{{ t('连接诊断报告') }}</label>
      <textarea id="connection-report-text" ref="field" :value="text" readonly spellcheck="false" />
      <button class="button secondary" @click="selectReport">{{ t('选中报告') }}</button>
    </details>
  </section>
</template>
