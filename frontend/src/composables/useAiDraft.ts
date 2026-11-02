// 「AI で下書き」の状態遷移（F-08）。キー未設定→設定へ誘導、未同意→同意、本文入力済み→置換確認、生成中→キャンセル可。
// 自動生成はしない: start() は利用者のボタン操作からのみ呼ぶ。結果は onGenerated に渡すだけで、失敗時は本文を変えない。
import { computed, ref } from 'vue'
import { type AppError, parseBindingError } from '../lib/api'

export type AiStep = 'idle' | 'consent' | 'replace' | 'generating'

export interface AiDraftDeps {
  /** 現在の AI 設定（未ロードなら undefined）。 */
  config: () =>
    | { provider: string; hasApiKey: boolean; consentedProviders: string[] | null }
    | undefined
  /** 署名・引用を除いた本文が入力済みか。 */
  hasBody: () => boolean
  recordConsent: (provider: string) => Promise<void>
  /** 生成。cancel() で Go 側の ctx をキャンセルする（requestId は呼び出しごとに一意）。 */
  generate: (requestId: string) => Promise<{ bodyHtml: string }>
  cancel: (requestId: string) => Promise<void>
  onGenerated: (bodyHtml: string) => void
  newRequestId: () => string
}

export function useAiDraft(deps: AiDraftDeps) {
  const step = ref<AiStep>('idle')
  const error = ref<string>()
  let requestId: string | undefined
  let cancelled = false

  // 判定は呼び出し時点の設定で行う（computed のキャッシュに頼らない）。テンプレート用は computed で公開する。
  function keyMissing(): boolean {
    return deps.config()?.hasApiKey !== true
  }
  function consentMissing(): boolean {
    const c = deps.config()
    return c !== undefined && !(c.consentedProviders ?? []).includes(c.provider)
  }
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
      if (!cancelled && err.code !== 'CANCELLED') error.value = err.message
    } finally {
      requestId = undefined
      step.value = 'idle'
    }
  }

  /** 次に必要な確認へ進む。 */
  async function proceed(): Promise<void> {
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
    await proceed()
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

  return { step, error, needsKey, needsConsent, start, answerConsent, answerReplace, cancel }
}
