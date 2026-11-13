import type { Entry, Preview, Rule } from './types'
export function splitName(name: string, kind: string): [string, string] {
  const i = name.lastIndexOf('.')
  return kind !== 'dir' && i > 0 ? [name.slice(0, i), name.slice(i)] : [name, '']
}
export function invalidName(name: string): string {
  if (!name || name === '.' || name === '..') return '文件名不能为空或为点号'
  if (/[\x00-\x1f<>:"/\\|?*]/u.test(name)) return '包含不兼容的文件名字符'
  if (/[. ]$/u.test(name)) return '名称不能以空格或点结尾'
  if (/^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(name)) return '系统保留名称'
  if (new TextEncoder().encode(name).length > 255) return '文件名超过 255 字节'
  return ''
}
export function preview(entries: Entry[], selected: string[], rules: Rule[]): Preview[] {
  const chosen = new Set(selected), existing = new Map(entries.map(e => [e.name.normalize('NFC').toLocaleLowerCase(), e.name]))
  const compiled = rules.filter(r => r.enabled).map(r => ({ ...r, re: r.kind === 'replace' && r.regex && r.find ? new RegExp(r.find, r.insensitive ? 'giu' : 'gu') : null }))
  let sequence = 0
  const rows: Preview[] = entries.map((entry, index) => {
    if (!chosen.has(entry.name)) return { name: entry.name, newName: entry.name, status: 'excluded', reason: '未选择', index }
    const n = sequence++
    let [stem, ext] = splitName(entry.name, entry.kind)
    for (const r of compiled) {
      switch (r.kind) {
        case 'replace':
          if (r.find) stem = r.re ? stem.replace(r.re, r.value) : r.insensitive ? stem.replace(new RegExp(r.find.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'giu'), () => r.value) : stem.split(r.find).join(r.value)
          break
        case 'affix': stem = r.prefix + stem + r.suffix; break
        case 'template': {
          const num = r.start + n * r.step
          if (!Number.isSafeInteger(num) || !Number.isInteger(r.pad) || r.pad < 1 || r.pad > 12) throw new Error('序号参数无效，补零位数需为 1 至 12')
          const values: Record<string, string> = { name: stem, n: String(num).padStart(r.pad, '0'), date: entry.modified.slice(0, 10).replaceAll('-', '') }
          if (/\{(?!name\}|n\}|date\})[^}]*\}/.test(r.value)) throw new Error('可用变量为 {name}、{n} 和 {date}')
          stem = r.value.replace(/\{(name|n|date)\}/g, (_, k: string) => values[k] ?? '')
          break
        }
        case 'case': stem = r.mode === 'upper' ? stem.toLocaleUpperCase() : stem.toLocaleLowerCase(); break
        case 'trim': stem = r.mode === 'all-spaces' ? stem.replace(/\s+/gu, '') : stem.trim().replace(/\s+/gu, ' '); break
        case 'extension': if (entry.kind !== 'dir') ext = r.value ? '.' + r.value.replace(/^\./, '') : ''; break
      }
    }
    const newName = stem + ext, reason = entry.kind === 'symlink' ? '符号链接不参与重命名' : invalidName(newName)
    const same = newName === entry.name
    const occupied = !same && existing.has(newName.normalize('NFC').toLocaleLowerCase())
    return { name: entry.name, newName, status: reason ? 'invalid' : same ? 'same' : occupied ? 'conflict' : 'ready', reason: reason || (same ? '名称未改变' : occupied ? '目标名称已存在（含大小写差异）' : ''), index }
  })
  const targets = new Map<string, Preview[]>()
  for (const row of rows) if (row.status === 'ready') { const k = row.newName.normalize('NFC').toLocaleLowerCase(); const list = targets.get(k) ?? []; list.push(row); targets.set(k, list) }
  for (const list of targets.values()) if (list.length > 1) for (const row of list) { row.status = 'conflict'; row.reason = '多个文件生成了相同名称' }
  return rows
}
