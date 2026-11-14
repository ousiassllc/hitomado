import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { AgentService } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/agentcore'
import type {
  AgentItem,
  AgentOverview,
  AgentSettings,
} from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { call, list, parseBindingError } from '../../lib/api'
import { EVENTS, onEvent } from '../../lib/events'
import { useUiStore } from '../ui'

/** AgentItem.status の値（Go の dto.AgentStatus*）。 */
export type AgentStatus = 'ready' | 'asking' | 'skipped' | 'later' | 'done' | 'failed'

/** 利用者の返事を待っている状態（確認待ち・質問待ち）。パネルの件数・バッジはこの合計。 */
const isPending = (i: AgentItem): boolean => i.status === 'ready' || i.status === 'asking'

/**
 * エージェントモード（#372）の設定と、扱った新着の一覧。Go の AgentService を呼ぶだけで、判断・下書き作りは Go 側が行う。
 * 送信に関わる操作はここに無い（送信は作成画面で人が押す）。
 */
export const useAgentStore = defineStore('agent', () => {
  const ui = useUiStore()
  const settings = ref<AgentSettings>({ enabled: false, mascotEnabled: false, aiReady: false })
  const overview = ref<AgentOverview>({ pendingCount: 0, items: [] })
  /** 処理中の項目 ID（行のボタンを無効にして二重送信を防ぐ）。 */
  const busyIds = ref<string[]>([])
  let loadSeq = 0
  let started = false
  let stops: (() => void)[] = []

  const enabled = computed(() => settings.value.enabled)
  const items = computed(() => list(overview.value.items))
  /** 確認待ち＋質問待ちの件数。オフの間は 0（ナビ・下タブのバッジを出さない）。 */
  const pendingCount = computed(() => (enabled.value ? overview.value.pendingCount : 0))

  function isBusy(id: string): boolean {
    return busyIds.value.includes(id)
  }

  /** 設定と一覧を読み直す。失敗はアラートで知らせ、投げ直さない。新しい読み込みが始まっていれば、古い応答は捨てる。 */
  async function load(): Promise<void> {
    const my = ++loadSeq
    try {
      const [s, o] = await Promise.all([
        call(AgentService.GetAgentSettings()),
        call(AgentService.ListAgentItems()),
      ])
      if (my !== loadSeq) return
      settings.value = s
      overview.value = { ...o, items: list(o.items) }
    } catch (e) {
      ui.pushAlert(parseBindingError(e).message)
    }
  }

  function applySettings(s: AgentSettings): void {
    loadSeq++ // 飛行中の読み込みは、これより古い設定を持っているので捨てる
    settings.value = s
  }

  /** エージェントのオン・オフ。オンにするには同意（agreed）と AI の設定が要り、満たさないと AppError を投げる（呼び出し側が案内する）。 */
  async function setEnabled(on: boolean, agreed: boolean): Promise<void> {
    applySettings(await call(AgentService.SetAgentEnabled(on, agreed)))
    await load()
  }

  async function setMascot(on: boolean): Promise<void> {
    applySettings(await call(AgentService.SetAgentMascot(on)))
  }

  /** Go が返した最新の 1 件で行を置き換え、件数を合わせる（イベントでの読み直しを待たずに画面を進める）。 */
  function replace(next: AgentItem): void {
    const found = items.value.some((i) => i.id === next.id)
    const merged = found
      ? items.value.map((i) => (i.id === next.id ? next : i))
      : [next, ...items.value]
    overview.value = { items: merged, pendingCount: merged.filter(isPending).length }
  }

  /** 項目への操作を 1 件ずつ行う（処理中は同じ項目を受け付けない）。失敗はアラートで知らせ、成功したかを返す。 */
  async function act(id: string, run: () => PromiseLike<AgentItem>): Promise<boolean> {
    if (isBusy(id)) return false
    busyIds.value = [...busyIds.value, id]
    try {
      replace(await call(run()))
      return true
    } catch (e) {
      ui.pushAlert(parseBindingError(e).message)
      return false
    } finally {
      busyIds.value = busyIds.value.filter((x) => x !== id)
    }
  }

  /** 質問への答え（質問と同じ順・同じ件数）を渡して、下書きを作る（作り直す）。 */
  const answer = (id: string, answers: string[]) =>
    act(id, () => AgentService.AnswerAgentQuestions(id, answers))
  /** 「あとで」。 */
  const defer = (id: string) => act(id, () => AgentService.DeferAgentItem(id))
  /** 下書きを開いて確認した。確認済み（done）にする。 */
  const confirm = (id: string) => act(id, () => AgentService.ConfirmAgentItem(id))
  /** 「やっぱり返信する」・失敗の「やり直す」。返信が要る前提で、下書き作り（必要なら質問）からやり直す。 */
  const reconsider = (id: string) => act(id, () => AgentService.ReconsiderAgentItem(id))

  /** 冪等。`agent:updated`（読み直し）と `agent:open-panel`（パネルを開く）を購読して、初期ロードする。 */
  async function start(): Promise<void> {
    if (started) return
    started = true
    stops = [
      onEvent(EVENTS.AGENT_UPDATED, () => void load()),
      onEvent(EVENTS.AGENT_OPEN_PANEL, () => ui.openAgent()),
    ]
    await load()
  }

  /** テスト・HMR 用の購読解除。 */
  function stop(): void {
    for (const s of stops) s()
    stops = []
    started = false
  }

  return {
    settings,
    overview,
    enabled,
    items,
    pendingCount,
    busyIds,
    isBusy,
    load,
    setEnabled,
    setMascot,
    answer,
    defer,
    confirm,
    reconsider,
    start,
    stop,
  }
})
