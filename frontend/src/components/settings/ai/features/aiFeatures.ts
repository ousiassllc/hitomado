// 「機能ごとに使う AI」（#376）の選択肢の組み立て。選べるのは、つないである AI（資格情報あり・同意済み）のモデルだけ。
// 選択肢の値は「プロバイダ|モデル」。TypeSafe AI（Jev）はモデルを持たないので「typesafe|」。
import type {
  AIConfig,
  AIFeatureSetting,
  AIModelList,
} from '../../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { aiModelsKey } from '../../../../lib/aiConfig'
import { list } from '../../../../lib/api'
import { settings as msg } from '../../../../lib/messages'
import type { SelectOption } from '../../../ui/BaseSelect.vue'
import { AI_PROVIDERS, isConnected } from '../providers/aiProviders'

/** 機能の ID（Go の dto.AIFeatureDraft／AIFeatureInboxZero と同じ）。 */
export type AIFeatureId = 'draft' | 'inboxZero'

/** 機能で選べるプロバイダ（表示順）。Jev は文章を書かないので「AI で下書き」には出さず、Inbox Zero だけ。 */
export function featureProviders(feature: AIFeatureId): string[] {
  return AI_PROVIDERS.map((p) => p.id).filter((id) => id !== 'typesafe' || feature === 'inboxZero')
}

/** そのプロバイダのモデル一覧の保存キー。OpenAI 互換は保存済みの Base URL ごと。 */
export function modelsKeyFor(provider: string, cfg: AIConfig): string {
  return aiModelsKey(provider, provider === 'openai' ? (cfg.baseUrl ?? '') : '')
}

const providerName = (p: string) => msg.ai.providerList.names[p] ?? p

export const optionValue = (provider: string, model: string) => `${provider}|${model}`

export function parseOption(value: string): { provider: string; model: string } {
  const i = value.indexOf('|')
  return { provider: value.slice(0, i), model: value.slice(i + 1) }
}

/** 選択肢 1 つの表示（「OpenAI 互換 ・ gpt-x」。Jev はサービス名だけ）。 */
export function optionLabel(provider: string, model: string, displayName?: string): string {
  if (provider === 'typesafe') return providerName(provider)
  const name = displayName && displayName !== model ? `${displayName}（${model}）` : model
  return `${providerName(provider)} ・ ${name}`
}

/** そのプロバイダの取得済みのモデル一覧（未取得は undefined）。エフォートの段階（モデルごと）を引くのに使う。 */
export function modelListFor(
  provider: string,
  cfg: AIConfig,
  models: Record<string, AIModelList>,
): AIModelList | undefined {
  return models[modelsKeyFor(provider, cfg)]
}

/** 一覧を取得できず、モデル名を手入力する、つないであるプロバイダ。message は取得できなかった理由（あれば）。 */
export interface ManualProvider {
  provider: string
  message?: string
}

export interface FeatureChoices {
  options: SelectOption[]
  manual: ManualProvider[]
  /** つないであるプロバイダのモデル一覧を、まだ取得している途中。 */
  loading: boolean
  /** 使える AI（つないであり、その機能で選べる）が 1 つも無い。 */
  noneAvailable: boolean
}

/**
 * feature の選択肢。つないでいない AI は出さない。保存済みの選択は、つなぎ直しが必要でも（一覧に無くても）残し、
 * 選択欄に表示できるよう末尾に足す（別の AI に置き換えない）。
 */
export function featureChoices(
  feature: AIFeatureId,
  cfg: AIConfig,
  models: Record<string, AIModelList>,
  selected: AIFeatureSetting,
): FeatureChoices {
  const options: SelectOption[] = []
  const manual: ManualProvider[] = []
  let loading = false
  const connected = featureProviders(feature).filter((p) => isConnected(p, cfg))
  for (const p of connected) {
    if (p === 'typesafe') {
      options.push({ value: optionValue(p, ''), label: optionLabel(p, '') })
      continue
    }
    const res = models[modelsKeyFor(p, cfg)]
    if (res === undefined) {
      loading = true
      continue
    }
    const listed = list(res.models)
    if (listed.length === 0) manual.push({ provider: p, message: res.message })
    for (const m of listed) {
      options.push({ value: optionValue(p, m.id), label: optionLabel(p, m.id, m.displayName) })
    }
  }
  if (selected.provider !== '') {
    const value = optionValue(selected.provider, selected.model)
    if (!options.some((o) => o.value === value)) {
      const label = optionLabel(selected.provider, selected.model)
      const listedProvider =
        isConnected(selected.provider, cfg) &&
        selected.provider !== 'typesafe' &&
        !manual.some((m) => m.provider === selected.provider)
      options.push({
        value,
        label: listedProvider && !loading ? msg.ai.modelNotListed(label) : label,
      })
    }
  }
  return { options, manual, loading, noneAvailable: connected.length === 0 }
}
