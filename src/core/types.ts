export interface Entry { name: string; kind: 'file' | 'dir' | 'symlink'; size: number; modified: string; fingerprint: string }
export interface Root { id: string; name: string }
export type RuleKind = 'replace' | 'template' | 'affix' | 'case' | 'trim' | 'extension'
export interface Rule { id: string; kind: RuleKind; enabled: boolean; find: string; value: string; regex: boolean; insensitive: boolean; prefix: string; suffix: string; start: number; step: number; pad: number; mode: string }
export interface Preview { name: string; newName: string; status: 'ready' | 'same' | 'conflict' | 'invalid' | 'excluded'; reason: string; index: number }
export interface Result { from: string; to: string; fingerprint: string; state: string; error?: string }
export interface Job { id: string; root: string; path: string; state: string; total: number; done: number; failed: number; started: string; finished?: string; items: Result[]; undoOf?: string }
export const ruleNames: Record<RuleKind, string> = { replace: '查找替换', template: '命名与序号', affix: '添加前后缀', case: '大小写转换', trim: '清理名称', extension: '修改扩展名' }
let nextRuleId = 0
export function makeRule(kind: RuleKind): Rule { return { id: `rule-${Date.now()}-${nextRuleId++}`, kind, enabled: true, find: '', value: kind === 'template' ? '{name}_{n}' : '', regex: false, insensitive: false, prefix: '', suffix: '', start: 1, step: 1, pad: 3, mode: kind === 'trim' ? 'spaces' : 'lower' } }
