import { preview } from './rename'
self.onmessage = e => { const t = performance.now(); try { self.postMessage({ rows: preview(e.data.entries, e.data.selected, e.data.rules), ms: performance.now() - t }) } catch (error) { self.postMessage({ error: error instanceof SyntaxError ? '正则表达式无效，请检查查找内容。' : error instanceof Error ? error.message : String(error) }) } }
