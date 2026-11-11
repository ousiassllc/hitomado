// Inbox Zero（#314）。ボタンを押すと、今のタブに読み込まれているメール（最大 100 件）を Go が判断して候補を返し、
// 候補を「差出人・件名」のチェックつき一覧で確認する。「アーカイブする（Yes）」のときだけ、チェックの入ったものをアーカイブする
// （アーカイブ・元に戻すは受信箱の通常のアーカイブと同じ。inbox.archive）。「やめる（No）」は何も変えない。
// Jev へ送るのは、初めて使うときに同意をもらってから（同意前は Go も送らない）。
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { AIService } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app'
import type {
  InboxZeroCandidate,
  InboxZeroReasons,
} from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { call, list, parseBindingError } from '../../lib/api'
import { keyOf, refOf } from '../../lib/inboxItems'
import { inboxZero as msg } from '../../lib/messages'
import { useInboxStore } from '../inbox'
import { useSettingsStore } from '../settings'
import { useUiStore } from '../ui'

/** 1 回に判断するメールの上限（Go も同じ値で検証する）。 */
export const INBOX_ZERO_MAX = 100
const PROVIDER = 'typesafe'

export type InboxZeroPhase = 'idle' | 'consent' | 'checking' | 'review'

export const useInboxZeroStore = defineStore('inboxZero', () => {
  const inbox = useInboxStore()
  const settings = useSettingsStore()
  const ui = useUiStore()

  /** 「不要」とみなす理由のオン・オフ。読み込み前・読み込めないときは Go の既定（4 つともオン）と同じ。 */
  const reasons = ref<InboxZeroReasons>({
    ad: true,
    notification: true,
    newsletter: true,
    replied: true,
  })
  const phase = ref<InboxZeroPhase>('idle')
  const candidates = ref<InboxZeroCandidate[]>([])
  const checked = ref<Set<string>>(new Set())
  const skipped = ref(0)
  let requestId: string | undefined

  /** Jev に聞く理由（広告・通知・ニュースレター）が 1 つでもオンか。返信済みだけなら Jev は要らない。 */
  const needsJev = computed(() => {
    const r = reasons.value
    return r.ad || r.notification || r.newsletter
  })
  const anyReason = computed(() => needsJev.value || reasons.value.replied)

  /** 押せない理由（押せるときは undefined）。ボタンはこの文言をツールチップとタップ時のトーストで出す。 */
  const blockedReason = computed<string | undefined>(() => {
    if (!anyReason.value) return msg.blockedNoReason
    if (inbox.items.length === 0) return msg.blockedEmpty
    const cfg = settings.aiConfig
    // 設定の読み込み前は止めない（押したときの検査は Go が行う）。
    if (needsJev.value && cfg && !list(cfg.apiKeyProviders).includes(PROVIDER)) {
      return msg.blockedNoKey
    }
    return undefined
  })

  const checkedCandidates = computed(() =>
    candidates.value.filter((c) => checked.value.has(keyOf(c.ref))),
  )

  /** 理由の設定を読み込む。失敗しても既定のまま（押したときの判断は Go が保存値で行う）。 */
  async function loadReasons(): Promise<void> {
    try {
      reasons.value = (await call(AIService.GetInboxZeroSettings())).reasons
    } catch {
      // 既定のまま
    }
  }

  /** 理由のオン・オフを保存する。保存に失敗したら今の値は変えずに投げる。 */
  async function setReasons(next: InboxZeroReasons): Promise<void> {
    reasons.value = (await call(AIService.SetInboxZeroReasons(next))).reasons
  }

  function reset(): void {
    phase.value = 'idle'
    candidates.value = []
    checked.value = new Set()
    skipped.value = 0
    requestId = undefined
  }

  /** ボタン。押せないときは理由をトーストで出す（タッチ UI はツールチップが出ないため）。 */
  async function start(): Promise<void> {
    if (phase.value !== 'idle') return
    const blocked = blockedReason.value
    if (blocked) {
      ui.pushToast('info', blocked)
      return
    }
    if (needsJev.value && !list(settings.aiConfig?.consentedProviders).includes(PROVIDER)) {
      phase.value = 'consent'
      return
    }
    await run()
  }

  /** 同意して続ける。同意を保存してから判断を始める。 */
  async function agree(): Promise<void> {
    if (phase.value !== 'consent') return
    try {
      await settings.recordAIConsent(PROVIDER)
    } catch (e) {
      reset()
      ui.pushAlert(parseBindingError(e).message)
      return
    }
    await run()
  }

  async function run(): Promise<void> {
    const id = crypto.randomUUID()
    requestId = id
    phase.value = 'checking'
    const refs = inbox.items.slice(0, INBOX_ZERO_MAX).map(refOf)
    try {
      const res = await call(AIService.FindInboxZeroCandidates(refs, id))
      if (requestId !== id) return // やめた（キャンセルした）あとの応答は捨てる
      const found = list(res.candidates)
      if (found.length === 0) {
        reset()
        ui.pushToast('info', msg.none(res.examined))
        return
      }
      candidates.value = found
      checked.value = new Set(found.map((c) => keyOf(c.ref))) // 最初は全部チェック済み
      skipped.value = res.skipped
      phase.value = 'review'
    } catch (e) {
      if (requestId !== id) return
      const err = parseBindingError(e)
      reset()
      if (err.code === 'CANCELLED') return
      if (err.code === 'AI_CONSENT_REQUIRED') {
        phase.value = 'consent'
        return
      }
      ui.pushAlert(err.message || msg.failed)
    }
  }

  function toggle(key: string): void {
    const next = new Set(checked.value)
    if (!next.delete(key)) next.add(key)
    checked.value = next
  }

  /** 「アーカイブする（Yes）」。チェックの入ったものだけをアーカイブし、件数と「元に戻す」のスナックバーは通常のアーカイブが出す。 */
  async function confirm(): Promise<void> {
    if (phase.value !== 'review') return
    const refs = checkedCandidates.value.map((c) => c.ref)
    if (refs.length === 0) return
    reset()
    await inbox.archive(refs)
  }

  /** 「やめる（No）」・閉じる・同意のキャンセル。何も変えない。判断の最中なら Go の呼び出しを中止する。 */
  function cancel(): void {
    const id = phase.value === 'checking' ? requestId : undefined
    reset()
    if (id) void call(AIService.CancelGenerate(id)).catch(() => undefined)
  }

  return {
    reasons,
    loadReasons,
    setReasons,
    phase,
    candidates,
    checked,
    skipped,
    needsJev,
    blockedReason,
    checkedCandidates,
    start,
    agree,
    toggle,
    confirm,
    cancel,
  }
})
