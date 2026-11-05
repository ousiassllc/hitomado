// 設定画面の AI の「プロバイダを選ぶ → 同意 → モデル一覧の取得」の流れ（#105）。選んだ送信先（保存前でも）に対して行う。
// 作成画面の初回の案内（#104）から入るときも、同じ流れを使う（AISettings が入口 `ui.aiSetupIntent` から refresh を呼ぶ）。
import type { Ref } from 'vue'
import type { AIModelList } from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { parseBindingError } from '../../../lib/api'
import { useSettingsStore } from '../../../stores/settings'
import type AIApiKey from './AIApiKey.vue'
import type AIChatGPTLogin from './AIChatGPTLogin.vue'

/** 処理中の種類（同時に 1 つだけ。undefined は処理中でない）。 */
export type AiBusy = 'config' | 'key' | 'clear' | 'models' | 'consent'

export interface AiSetupFlowDeps {
  provider: Ref<string>
  draftBaseUrl: Ref<string>
  isChatGPT: Ref<boolean>
  busy: Ref<AiBusy | undefined>
  errors: Ref<{ model?: string; config?: string; key?: string; consent?: string }>
  consentOpen: Ref<boolean>
  keyForm: Ref<InstanceType<typeof AIApiKey> | undefined>
  chatgptLogin: Ref<InstanceType<typeof AIChatGPTLogin> | undefined>
}

export function useAiSetupFlow(deps: AiSetupFlowDeps) {
  const store = useSettingsStore()
  const { provider, draftBaseUrl, isChatGPT, busy, errors, consentOpen, keyForm, chatgptLogin } =
    deps
  let loadSeq = 0

  /**
   * 選んだ送信先のモデル一覧を取得する。取得できないときは理由が一覧の注記に出て、手入力のままになる（エラーにはしない）。
   * 取得は重なり得る（続けてプロバイダを切り替える・取得中に「一覧を更新」）ので、取得ごとに世代番号を持つ。
   * 処理中の表示（busy）は最新の取得が終わるまで保ち、古い取得の結果・エラーは捨てる（undefined を返す）。
   */
  async function loadModels(): Promise<AIModelList | undefined> {
    if (busy.value && busy.value !== 'models') return undefined
    const seq = ++loadSeq
    busy.value = 'models'
    try {
      const r = await store.loadAIModels(provider.value, draftBaseUrl.value)
      return seq === loadSeq ? r : undefined
    } catch (e) {
      if (seq === loadSeq) errors.value = { ...errors.value, model: parseBindingError(e).message }
      return undefined
    } finally {
      if (seq === loadSeq) busy.value = undefined
    }
  }

  /**
   * 取得のあとの案内。資格情報が無いとき、ChatGPT は「Continue with ChatGPT」のログインへ進み（login=true のとき自動で始める）、
   * API キーのプロバイダはキーの入力欄へ誘導する。
   */
  function guideCredentials(r: AIModelList, login: boolean): void {
    if (r.reason !== 'no_credentials') return
    if (isChatGPT.value) {
      if (login) void chatgptLogin.value?.login()
      return
    }
    keyForm.value?.focus()
  }

  /**
   * プロバイダ・送信先を選んだときの流れの入口。まず取得を試し、同意前の送信先なら（Go が取得を送らず consent_required を返すので）
   * 同意の画面を出す。同意済みならそのまま一覧が出る。login は、資格情報が無い ChatGPT のログインを自動で始めるか。
   */
  async function refresh(login = false): Promise<void> {
    const [p, url] = [provider.value, draftBaseUrl.value]
    const r = await loadModels()
    if (!r || p !== provider.value || url !== draftBaseUrl.value) return // 取得中に選び直された
    if (r.reason === 'consent_required') {
      consentOpen.value = true
      return
    }
    guideCredentials(r, login)
  }

  /** 同意の画面の「同意する」。同意を保存し、すぐ一覧を取得する。ChatGPT で未ログインなら、続けてログインへ進む。 */
  async function agree(): Promise<void> {
    if (busy.value) return
    errors.value = {}
    busy.value = 'consent'
    const [p, url] = [provider.value, draftBaseUrl.value]
    try {
      await store.recordAIConsent(p, url)
    } catch (e) {
      errors.value = { consent: parseBindingError(e).message }
      consentOpen.value = false
      return
    } finally {
      busy.value = undefined
    }
    consentOpen.value = false
    const r = await loadModels()
    if (!r || p !== provider.value || url !== draftBaseUrl.value) return
    guideCredentials(r, true)
  }

  /** 閉じた（同意しなかった）とき。取得はせず、プロバイダの選択だけが残る（モデル名は手入力できる）。 */
  function declineConsent(): void {
    consentOpen.value = false
  }

  return { refresh, agree, declineConsent }
}
