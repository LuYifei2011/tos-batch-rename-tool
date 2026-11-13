import { preview } from './rename.ts'
import { makeRule } from './types.ts'
import type { Entry } from './types.ts'
const entries: Entry[] = Array.from({ length: 50000 }, (_, i) => ({ name: `IMG_${String(i).padStart(6,'0')}.jpg`, kind: 'file', size: 1024, modified: '2026-11-10T00:00:00Z', fingerprint: 'benchmark' }))
const r=makeRule('template');r.value='照片_{date}_{n}';const selected=entries.map(e=>e.name)
for(let i=0;i<3;i++) preview(entries,selected,[r])
const results=[];for(let i=0;i<7;i++){const start=performance.now();const rows=preview(entries,selected,[r]);results.push(performance.now()-start);if(rows.filter(r=>r.status==='ready').length!==50000)throw new Error('invalid benchmark result')}
results.sort((a,b)=>a-b);console.log(JSON.stringify({scenario:'50,000 names, date and sequence, full conflict validation',runs:7,medianMs:results[3],minMs:results[0],maxMs:results[6],scope:'pure computation; excludes worker transfer, disk and DOM'},null,2))
