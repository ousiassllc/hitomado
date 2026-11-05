// 「AI で下書き」の状態遷移（F-08）。AI の設定が未保存→初回の案内（ChatGPT でログイン／別のプロバイダ）、キー未設定→設定へ誘導、
// 未同意→同意、本文入力済み→置換確認、生成中→キャンセル可。
// 自動生成はしない: start() は利用者のボタン操作からのみ呼ぶ。結果は onGenerated に渡すだけで、失敗時は本文を変えない。
import { computed, ref } from 'vue'
import { type AppError, parseBindingError } from '../lib/api'

export type AiStep = 'idle' | 'setup' | 'consent' | 'replace' | 'generating'

/** 初回の案内の選択。chatgpt は ChatGPT でログインする流れ、other は別のプロバイダを選ぶ（どちらも設定画面の AI の流れへ進む）。 */
export type AiSetupChoice = 'chatgpt' | 'other'

export interface AiDraftDeps {
  /** 現在の AI 設定（未ロードなら undefined）。 */
  config: () =>
    | {
        provider: string
        configured: boolean
        hasApiKey: boolean
        consentedProviders: string[] | null
      }
    | undefined
  /** 初回の案内から設定画面の AI の流れへ進む（ChatGPT のログイン、または別のプロバイダの選択）。 */
  openSetup: (choice: AiSetupChoice) => void
  /** 署名・引用を除いた本文が入力済みか。 */
  hasBody: () => boolean
  recordConsent: (provider: string) => Promise<void>
  /** 生成。cancel() で Go 側の ctx をキャンセルする（requestId は呼び出しごとに一意）。 */
  generate: (requestId: string) => Promise<{ bodyHtml: string }>
  /** 「AI への指示」が使えないときの理由（上限超過など）。あれば生成を始めずにエラー表示する（本文は変えない）。 */
  instructionError?: () => string | undefined
  cancel: (requestId: string) => Promise<void>
  onGenerated: (bodyHtml: string) => void
  newRequestId: () => string
}

/** ChatGPT プランの利用枠の超過か（Go が AI_FAILED に付ける details.reason。メッセージの文言には頼らない）。 */
export function isUsageLimit(err: AppError): boolean {
  return err.code === 'AI_FAILED' && err.details?.reason === 'usage_limit_exceeded'
}

export function useAiDraft(deps: AiDraftDeps) {
  const step = ref<AiStep>('idle')
  const error = ref<string>()
  /** ChatGPT プランの利用枠の超過（AI_FAILED の details.reason）。error の代わりに、使用状況の画面を開く導線つきの案内を出す。 */
  const usageLimit = ref(false)
  let requestId: string | undefined
  let cancelled = false

  // 判定は呼び出し時点の設定で行う（computed のキャッシュに頼らない）。テンプレート用は computed で公開する。
  /** AI の設定をまだ保存していない（初めて使う）。保存前の既定のプロバイダではモデルも決まっていないので、先に初回の案内へ進める。 */
  function setupMissing(): boolean {
    return deps.config()?.configured === false
  }
  function keyMissing(): boolean {
    return !setupMissing() && deps.config()?.hasApiKey !== true
  }
  function consentMissing(): boolean {
    const c = deps.config()
    return c !== undefined && !(c.consentedProviders ?? []).includes(c.provider)
  }
  const needsSetup = computed(setupMissing)
  const needsKey = computed(keyMissing)
  const needsConsent = computed(consentMissing)

  async function generate(): Promise<void> {
    const id = deps.newRequestId()
    requestId = id
    cancelled = false
    step.value = 'generating'
    try {
      const r = await deps.generate(id)
      if (!cancelled) deps.onGenerated(r.bodyHtml)
    } catch (e) {
      const err: AppError = parseBindingError(e)
      if (cancelled || err.code === 'CANCELLED') return
      if (isUsageLimit(err)) usageLimit.value = true
      else error.value = err.message
    } finally {
      requestId = undefined
      step.value = 'idle'
    }
  }

  /** 次に必要な確認へ進む。 */
  async function proceed(): Promise<void> {
    if (setupMissing()) {
      step.value = 'setup'
      return
    }
    if (keyMissing()) return // ボタン自体を無効にして設定へ誘導する（呼ばれない想定の防御）
    if (consentMissing()) {
      step.value = 'consent'
      return
    }
    if (deps.hasBody()) {
      step.value = 'replace'
      return
    }
    await generate()
  }

  /** 「AI で下書き」ボタン。 */
  async function start(): Promise<void> {
    if (step.value !== 'idle') return
    error.value = undefined
    usageLimit.value = false
    const invalid = deps.instructionError?.()
    if (invalid !== undefined) {
      error.value = invalid
      return
    }
    await proceed()
  }

  /** 初回の案内の結果。choice が無い（あとで）は何もしない。進むときは設定画面へ移り、下書きはまだ作らない（もう一度ボタンを押す）。 */
  function answerSetup(choice?: AiSetupChoice): void {
    if (step.value !== 'setup') return
    step.value = 'idle'
    if (choice !== undefined) deps.openSetup(choice)
  }

  /** 同意ダイアログの結果。同意すると記録して続行する。 */
  async function answerConsent(agree: boolean): Promise<void> {
    if (step.value !== 'consent') return
    const provider = deps.config()?.provider
    if (!agree || provider === undefined) {
      step.value = 'idle'
      return
    }
    try {
      await deps.recordConsent(provider)
    } catch (e) {
      error.value = parseBindingError(e).message
      step.value = 'idle'
      return
    }
    step.value = 'idle'
    await proceed()
  }

  /** 置換確認の結果。 */
  async function answerReplace(ok: boolean): Promise<void> {
    if (step.value !== 'replace') return
    if (!ok) {
      step.value = 'idle'
      return
    }
    await generate()
  }

  /** 生成中のキャンセル。 */
  async function cancel(): Promise<void> {
    const id = requestId
    if (step.value !== 'generating' || id === undefined) return
    cancelled = true
    try {
      await deps.cancel(id)
    } catch {
      // 既に完了している場合などは無視する（結果は cancelled で捨てる）
    }
  }

  return {
    step,
    error,
    usageLimit,
    needsSetup,
    needsKey,
    needsConsent,
    start,
    answerSetup,
    answerConsent,
    answerReplace,
    cancel,
  }
}
