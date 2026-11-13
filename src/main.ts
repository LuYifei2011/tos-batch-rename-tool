import { createApp } from 'vue'
import App from './App.vue'
import './style.css'
import { initializePreferences } from './core/preferences'
const disposePreferences = initializePreferences()
const app = createApp(App)
app.onUnmount(disposePreferences)
app.mount('#app')
if (import.meta.hot) import.meta.hot.dispose(disposePreferences)
