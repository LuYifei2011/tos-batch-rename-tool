<script setup lang="ts">
import { t } from '../core/i18n'
import { languagePreference, themePreference } from '../core/preferences'
</script>

<template>
  <div class="preference-fields">
    <label class="field-label">{{ t('语言') }}<select v-model="languagePreference" :aria-label="t('语言')"><option value="tos">{{ t('跟随 TOS') }}</option><option value="browser">{{ t('跟随浏览器') }}</option><option value="zh-CN" lang="zh-CN">简体中文</option><option value="en-US" lang="en">English</option></select></label>
    <label class="field-label">{{ t('主题') }}<select v-model="themePreference" :aria-label="t('主题')"><option value="tos">{{ t('跟随 TOS') }}</option><option value="browser">{{ t('跟随浏览器') }}</option><option value="light">{{ t('浅色') }}</option><option value="dark">{{ t('深色') }}</option></select></label>
  </div>
</template>
