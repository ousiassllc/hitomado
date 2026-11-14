import { createPinia } from 'pinia'
import { createApp } from 'vue'
import { createErrorReporter } from './lib/errorReporter'

// 画面の端のマスコット（別ウィンドウ。URL は /#mascot）は、通常のアプリ（ストア群・3 ペイン）を読み込まず、小さな専用のページだけを描く。
async function start(): Promise<void> {
  const isMascot = location.hash === '#mascot'
  const { default: Root } = isMascot
    ? await import('./mascot/MascotApp.vue')
    : await import('./App.vue')
  const app = createApp(Root)
  // マウント前に取り付ける（起動時のエラーも拾う）。
  createErrorReporter().install(app)
  if (!isMascot) app.use(createPinia())
  app.mount('#app')
}

void start()
