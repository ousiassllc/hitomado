// スレッドの AI 要約（F-17）・質問（F-18）の状態遷移。useAiDraft と同じ前提確認（lib/aiPrecheck）を、「本文入力済み→置換確認」を除いて行う:
// AI の設定が未保存→初回の案内、キー未設定→設定へ誘導（ボタンを無効にする）、未同意→同意の画面。
// 自動では呼ばない: start() は利用者の操作（「AI で要約」ボタン／質問の送信）からのみ呼ぶ。同意を取り直して同意されたら、押された操作をそのまま続ける。
import { computed, ref } from 'vue'
import {
  type AiPrecheckConfig,
  aiConsentMissing,
  aiKeyMissing,
  aiPrecheck,
  aiSetupMissing,
} from '../lib/aiPrecheck'
import { parseBindingError } from '../lib/api'
import type { AiSetupChoice } from './useAiDraft'

export type ThreadAiStep = 'idle' | 'setup' | 'consent'
/** 前提確認のあとに続ける操作。 */
export type ThreadAiAction = 'summarize' | 'ask'

export interface ThreadAiDeps {
  /** 現在の AI 設定（未ロードなら undefined）。 */
  config: () => AiPrecheckConfig | undefined
  /** 初回の案内から設定画面の AI の流れへ進む。 */
  openSetup: (choice: AiSetupChoice) => void
  recordConsent: (provider: string) => Promise<void>
  /** 前提確認を通ったあとの実処理（threadAiStore の summarize／ask）。 */
  run: (action: ThreadAiAction) => Promise<void>
}

export function useThreadAi(deps: ThreadAiDeps) {
  const step = ref<ThreadAiStep>('idle')
  /** 同意の記録の失敗など、生成の前に起きたエラー（生成の失敗は threadAiStore.error）。 */
  const error = ref<string>()
  let pending: ThreadAiAction | undefined

  // 判定は呼び出し時点の設定で行う。テンプレート用は computed で公開する。
  const needsSetup = computed(() => aiSetupMissing(deps.config()))
  const needsKey = computed(() => aiKeyMissing(deps.config()))
  const needsConsent = computed(() => aiConsentMissing(deps.config()))

  /** 次に必要な確認へ進み、すべて済んでいれば pending の操作を実行する。 */
  async function proceed(): Promise<void> {
    const action = pending
    if (action === undefined) return
    switch (aiPrecheck(deps.config())) {
      case 'setup':
        step.value = 'setup'
        return
      case 'key':
        pending = undefined // ボタン自体を無効にして設定へ誘導する（呼ばれない想定の防御）
        return
      case 'consent':
        step.value = 'consent'
        return
      case 'ready':
        pending = undefined
        await deps.run(action)
    }
  }

  /** 「AI で要約」ボタン・質問の送信。確認の途中（setup／consent）では何もしない。 */
  async function start(action: ThreadAiAction): Promise<void> {
    if (step.value !== 'idle') return
    error.value = undefined
    pending = action
    await proceed()
  }

  /** 初回の案内の結果。choice が無い（あとで）は何もしない。進むときは設定画面へ移り、操作はまだ実行しない（もう一度押す）。 */
  function answerSetup(choice?: AiSetupChoice): void {
    if (step.value !== 'setup') return
    step.value = 'idle'
    pending = undefined
    if (choice !== undefined) deps.openSetup(choice)
  }

  /** 同意ダイアログの結果。同意すると記録して、押された操作を続ける。 */
  async function answerConsent(agree: boolean): Promise<void> {
    if (step.value !== 'consent') return
    const provider = deps.config()?.provider
    if (!agree || provider === undefined) {
      step.value = 'idle'
      pending = undefined
      return
    }
    try {
      await deps.recordConsent(provider)
    } catch (e) {
      error.value = parseBindingError(e).message
      step.value = 'idle'
      pending = undefined
      return
    }
    step.value = 'idle'
    await proceed()
  }

  return { step, error, needsSetup, needsKey, needsConsent, start, answerSetup, answerConsent }
}
