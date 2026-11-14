import { computed, onMounted } from 'vue'
import { useThreadAi } from '../../../composables/useThreadAi'
import { thread as msg } from '../../../lib/messages'
import { useSettingsStore } from '../../../stores/settings'
import { useThreadStore } from '../../../stores/thread'
import { useThreadAiStore } from '../../../stores/threadAi'
import { useUiStore } from '../../../stores/ui'
import { draftPrecheckConfig } from '../../settings/ai/aiPrecheckConfig'

/** スレッドの AI 要約・質問（F-17・F-18）の前提確認を、設定・スレッド・threadAiStore につないだもの。`ThreadPane` が 1 つだけ持つ。 */
export function useThreadAiFlow() {
  const settings = useSettingsStore()
  const thread = useThreadStore()
  const store = useThreadAiStore()
  const ui = useUiStore()

  onMounted(() => {
    if (settings.aiConfig === undefined) void settings.load()
  })

  const flow = useThreadAi({
    config: () => draftPrecheckConfig(settings.aiConfig),
    // 同意は、保存済みの設定の送信先（OpenAI 互換は Base URL も）に対して記録する。
    recordConsent: (provider) =>
      settings.recordAIConsent(provider, settings.aiConfig?.baseUrl ?? ''),
    openSetup: (choice) =>
      ui.openSettings('ai', choice === 'chatgpt' ? 'chatgpt-login' : undefined),
    run: async (action) => {
      const ref = thread.currentRef
      if (!ref) return
      if (action === 'summarize') await store.summarize(ref)
      else await store.ask(ref, store.draft)
    },
  })

  // 「AI で下書き」に選んだのが ChatGPT プランのときだけ、ログインの案内にする（案内文の出し分けだけに使う）。
  const usingChatGPT = computed(() => settings.aiConfig?.features.draft.provider === 'chatgpt')

  /** 「AI で要約」を押せない（生成中・API キー未設定）。 */
  const summarizeDisabled = computed(() => store.busy || flow.needsKey.value)
  /** キー未設定のとき、ボタンのツールチップに出す案内。 */
  const summarizeHint = computed(() =>
    flow.needsKey.value ? (usingChatGPT.value ? msg.ai.needLogin : msg.ai.needKey) : undefined,
  )

  return { ...flow, usingChatGPT, summarizeDisabled, summarizeHint }
}
