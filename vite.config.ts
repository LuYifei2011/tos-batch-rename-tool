import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { readFileSync } from 'node:fs'
const version = JSON.parse(readFileSync(new URL('./package.json', import.meta.url), 'utf8')).version as string
export default defineConfig({
  plugins: [vue(), { name: 'app-build-info', transformIndexHtml: () => [{ tag: 'meta', attrs: { name: 'batch-rename-version', content: version }, injectTo: 'head' }] }], base: './',
  define: { __APP_VERSION__: JSON.stringify(version) },
  server: { proxy: { '/api': 'http://127.0.0.1:18946', '/v2/proxy/batch-rename': 'http://127.0.0.1:18946' } },
  build: { target: 'es2022', sourcemap: false, assetsInlineLimit: 0 },
})
