import { createPinia } from 'pinia'
import { createApp } from 'vue'
import App from './App.vue'
import { createErrorReporter } from './lib/errorReporter'

const app = createApp(App)
// マウント前に取り付ける（起動時のエラーも拾う）。
createErrorReporter().install(app)
app.use(createPinia()).mount('#app')
