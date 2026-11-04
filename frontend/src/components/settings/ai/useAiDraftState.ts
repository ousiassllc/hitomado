// 設定画面の AI 設定の、編集中（保存前）の選択から導く状態。保存済みの設定（store.aiConfig）とは独立に、選んだプロバイダ・
// Base URL の資格情報・一覧・同意を扱う（保存前でも取得・同意できる。#105）。
import { computed, type Ref, watch } from 'vue'
import { aiModelsKey } from '../../../lib/aiConfig'
import { settings as msg } from '../../../lib/messages'
import { useSettingsStore } from '../../../stores/settings'
import { aiDestination, DEFAULT_AI_PROVIDER, providerLabel } from './aiDestination'

export function useAiDraftState(provider: Ref<string>, model: Ref<string>, baseUrl: Ref<string>) {
  const store = useSettingsStore()
  const saved = computed(() => store.aiConfig)

  function resetDraft(): void {
    provider.value = saved.value?.provider ?? DEFAULT_AI_PROVIDER
    model.value = saved.value?.model ?? ''
    baseUrl.value = saved.value?.baseUrl ?? ''
  }
  // 下書きは、保存済みの設定そのものが変わったときだけ戻す。同意・キー・ログインで aiConfig が更新されても、選びかけのプロバイダは変えない。
  const savedDraftKey = computed(() =>
    JSON.stringify([
      saved.value?.configured,
      saved.value?.provider,
      saved.value?.model,
      saved.value?.baseUrl,
    ]),
  )
  watch(savedDraftKey, resetDraft, { immediate: true })

  const isChatGPT = computed(() => provider.value === 'chatgpt')
  const draftBaseUrl = computed(() => (provider.value === 'openai' ? baseUrl.value.trim() : ''))
  const baseUrlChanged = computed(() => draftBaseUrl.value !== (saved.value?.baseUrl ?? ''))
  const providerChanged = computed(() => provider.value !== saved.value?.provider)
  const configDirty = computed(
    () =>
      saved.value?.configured !== true ||
      providerChanged.value ||
      model.value.trim() !== (saved.value?.model ?? '') ||
      baseUrlChanged.value,
  )
  const draftInSync = computed(
    () => saved.value !== undefined && !providerChanged.value && !baseUrlChanged.value,
  )

  // 選んだ送信先（プロバイダ・OpenAI 互換は Base URL）の資格情報・一覧。保存済みの設定とは独立に扱う。
  const hasCredential = computed(() =>
    isChatGPT.value
      ? (saved.value?.chatgptSignedIn ?? false)
      : (saved.value?.apiKeyProviders?.includes(provider.value) ?? false),
  )
  const models = computed(() => store.aiModels[aiModelsKey(provider.value, draftBaseUrl.value)])
  // 選んだ送信先への同意。保存済みの設定と同じ送信先なら保存済みの状態、そうでなければ最後の取得の結果（未取得・Base URL が使えないときは不明）。
  const draftConsent = computed<boolean | undefined>(() => {
    if (draftInSync.value) return saved.value?.consentedProviders?.includes(provider.value) ?? false
    const m = models.value
    if (m === undefined || m.reason === 'invalid_base_url') return undefined
    return m.reason !== 'consent_required'
  })
  const draftDestination = computed(() => aiDestination(provider.value, draftBaseUrl.value))
  const consentText = computed(() => {
    const label = providerLabel(provider.value)
    if (draftConsent.value === true) return msg.ai.consentGiven(label)
    return draftConsent.value === false ? msg.ai.consentNeeded(label) : msg.ai.consentUnknown(label)
  })
  // OpenAI 互換は Base URL が決まってから取得する。入力済み、または保存済みの設定がすでに既定の送信先のときは決まっている。
  const destinationDecided = computed(
    () =>
      provider.value !== 'openai' ||
      draftBaseUrl.value !== '' ||
      (draftInSync.value && saved.value?.configured === true),
  )

  return {
    saved,
    isChatGPT,
    draftBaseUrl,
    providerChanged,
    configDirty,
    hasCredential,
    models,
    draftConsent,
    draftDestination,
    consentText,
    destinationDecided,
  }
}
