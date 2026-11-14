import { createTosApp } from 'tos-app-core'
import { createTosAppVue } from 'tos-app-core/vue'
import { APP_VERSION } from './build.ts'

export const tosApp = createTosApp({
  id: 'batch-rename', version: APP_VERSION,
  api: {
    timeoutMs: 65000,
    versionHeader: 'X-Batch-Rename-Version',
    blockOnAccessFailure: true,
    diagnosticPath: path => /^\/(info|list|plans|execute|undo|cancel|jobs)$/.test(path) ? path : /^\/jobs\/[a-f0-9]{32}$/.test(path) ? '/jobs/:id' : '/:endpoint',
    onAccessFailure: error => { if (error.kind === 'session') location.assign('/tos/') },
  },
})
export const tos = createTosAppVue(tosApp)
