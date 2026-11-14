import { createApp } from 'vue'
import App from './App.vue'
import './style.css'
import { initializePreferences } from './core/preferences'
import { tos } from './core/app'
initializePreferences()
const app = createApp(App)
app.use(tos)
app.mount('#app')
if (import.meta.hot) import.meta.hot.dispose(() => app.unmount())
