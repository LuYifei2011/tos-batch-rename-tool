import { test } from 'node:test'
import assert from 'node:assert/strict'
import { preview, invalidName, splitName } from './rename.ts'
import { makeRule } from './types.ts'
import type { Entry, Rule } from './types.ts'
const entry = (name: string): Entry => ({ name, kind: 'file', size: 0, modified: '2026-11-10T00:00:00Z', fingerprint: 'test' })
function run(names: string[], rules: Rule[], selected = names) { return preview(names.map(entry), selected, rules) }
test('dotfiles and multiple dots preserve the intended extension', () => { assert.deepEqual(splitName('.env', 'file'), ['.env', '']); assert.deepEqual(splitName('archive.tar.gz', 'file'), ['archive.tar', '.gz']) })
test('composed rules apply in order and preserve extensions', () => { const r=makeRule('replace'); r.find='IMG'; r.value='Photo'; const a=makeRule('affix'); a.prefix='旅行_'; assert.equal(run(['IMG_001.jpg'], [r,a])[0]!.newName, '旅行_Photo_001.jpg') })
test('literal replacements do not interpret regex or dollar tokens', () => { const r=makeRule('replace'); r.find='[x]'; r.value='$&'; assert.equal(run(['[x].txt'], [r])[0]!.newName, '$&.txt'); r.insensitive=true; assert.equal(run(['[X].txt'], [r])[0]!.newName, '$&.txt') })
test('regular expression capture groups are supported', () => { const r=makeRule('replace');r.regex=true;r.find='(IMG)_(\\d+)';r.value='$2-$1';assert.equal(run(['IMG_001.jpg'],[r])[0]!.newName,'001-IMG.jpg') })
test('sequence increments only for selected entries', () => { const r=makeRule('template');r.value='照片_{n}_{date}';assert.deepEqual(run(['a.jpg','b.jpg','c.jpg'],[r],['a.jpg','c.jpg']).map(x=>x.newName),['照片_001_20261110.jpg','b.jpg','照片_002_20261110.jpg']) })
test('duplicate targets are conflicts', () => { const r=makeRule('template');r.value='same';assert.ok(run(['a.jpg','b.jpg'],[r]).every(x=>x.status==='conflict')) })
test('unselected files still block occupied targets', () => { const r=makeRule('template');r.value='b';assert.equal(run(['a.jpg','b.jpg'],[r],['a.jpg'])[0]!.status,'conflict') })
test('case-only and canonical-equivalent collisions are blocked', () => { const r=makeRule('case');r.mode='lower';assert.equal(run(['A.JPG'],[r])[0]!.status,'conflict'); const a=makeRule('affix');a.prefix='é';assert.equal(run(['a.txt','e\u0301a.txt'],[a],['a.txt'])[0]!.status,'conflict') })
test('invalid patterns and sequence parameters are reported', () => { const r=makeRule('replace');r.regex=true;r.find='(';assert.throws(()=>run(['a.txt'],[r]));const n=makeRule('template');n.pad=100000;assert.throws(()=>run(['a.txt'],[n])) })
test('reserved names and byte length are validated', () => { for (const n of ['CON.txt','x/../y','..','x.','x ','😀'.repeat(65),'a\x00b']) assert.ok(invalidName(n));assert.equal(invalidName('会议记录.txt'),'') })
test('extension changes do not affect folder names', () => {const r=makeRule('extension');r.value='png';assert.equal(run(['a.jpg'],[r])[0]!.newName,'a.png');assert.equal(preview([{...entry('a.folder'),kind:'dir'}],['a.folder'],[r])[0]!.newName,'a.folder')})
test('symbolic links are never executable',()=>{const r=makeRule('affix');r.prefix='x';assert.equal(preview([{...entry('a'),kind:'symlink'}],['a'],[r])[0]!.status,'invalid')})
