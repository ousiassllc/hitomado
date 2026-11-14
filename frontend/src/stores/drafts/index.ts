// 下書きの一覧（F-12。#366）。サイドバーの「下書き」を開いている間、スレッド一覧の場所に全アカウントの Gmail の下書きを出す。
// 一覧は Gmail の下書きそのもの（ListDrafts）で、アプリでは保存しない。表示するアカウントは受信箱と同じ絞り込み（inbox.filter）に従う。
import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { ComposeService } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app'
import type { DraftSummary } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { call, list, parseBindingError } from '../../lib/api'
import { applyErrorEffects } from '../../lib/errorEffects'
import { drafts as msg } from '../../lib/messages'
import { useAccountsStore } from '../accounts'
import { useComposeStore } from '../compose'
import { useInboxStore } from '../inbox'
import { type Failure, toFailures } from '../inboxFailures'
import { useThreadStore } from '../thread'
import { useUiStore } from '../ui'

/** アカウントごとに読む上限（Go の draftListLimit と同じ。切り捨てた旨の文言に使う）。 */
export const DRAFT_LIST_LIMIT = 100

export const useDraftsStore = defineStore('drafts', () => {
  const ui = useUiStore()
  const accounts = useAccountsStore()
  const inbox = useInboxStore()
  const compose = useComposeStore()
  const thread = useThreadStore()

  /** 下書きの一覧を開いている（スレッド一覧の場所に出す）。 */
  const active = ref(false)
  const items = ref<DraftSummary[]>([])
  const loading = ref(false)
  const truncated = ref(false)

  let generation = 0

  /** 一覧を読み直す。新しい読み込みが始まったら、古い応答は捨てる。 */
  async function load(): Promise<void> {
    const gen = ++generation
    loading.value = true
    try {
      const r = await call(ComposeService.ListDrafts([...inbox.filter.accountIds]))
      if (gen !== generation) return
      items.value = list(r.items).map((d) => ({ ...d, to: list(d.to) }))
      truncated.value = r.truncated
      for (const f of toFailures(r.failures)) reportFailure(f)
    } catch (e) {
      if (gen !== generation) return
      const err = parseBindingError(e)
      applyErrorEffects(err, { setOnline: ui.setOnline, markNeedsReauth: accounts.markNeedsReauth })
      if (err.code !== 'OFFLINE') ui.pushAlert(msg.loadFailedAll, () => void load())
    } finally {
      if (gen === generation) loading.value = false
    }
  }

  /** 1 アカウントの取得失敗。要再接続・オフラインは共通の表示（バナー）に任せ、それ以外は再試行つきで知らせる。 */
  function reportFailure(f: Failure): void {
    if (f.code === 'NEEDS_REAUTH') accounts.markNeedsReauth(f.accountId)
    else if (f.code === 'OFFLINE') ui.setOnline(false)
    else
      ui.pushAlert(msg.loadFailed(accounts.byId(f.accountId)?.displayName ?? ''), () => void load())
  }

  function show(): void {
    active.value = true
    void load()
  }

  function hide(): void {
    active.value = false
  }

  /** 行を開く。返信の下書きは元のスレッドの返信として（スレッドも開く）、新規の下書きは作成画面で復元する（F-12）。 */
  async function open(d: DraftSummary): Promise<boolean> {
    if (!d.isReply) return compose.open({ mode: 'new', accountId: d.accountId, draftId: d.draftId })
    const ref = { accountId: d.accountId, threadId: d.threadId }
    inbox.select(ref)
    return compose.open({ mode: 'reply_all', ref, draftId: d.draftId })
  }

  /** 開いている作成セッションがこの下書きのものか。 */
  function isOpen(d: DraftSummary): boolean {
    const s = compose.session
    return s !== undefined && s.draftId === d.draftId && s.accountId === d.accountId
  }

  /**
   * 下書きを破棄する（Gmail の下書きも消える。確認は呼び出し側）。開いている作成セッションの下書きは、セッションごと破棄する
   * （そうしないと、開いたままの作成画面が下書きを作り直してしまう）。成功したら true。
   */
  async function discard(d: DraftSummary): Promise<boolean> {
    if (isOpen(d)) return compose.discard()
    try {
      await call(
        ComposeService.DiscardDraft({
          accountId: d.accountId,
          draftId: d.draftId,
          threadId: d.threadId,
        }),
      )
    } catch (e) {
      const err = parseBindingError(e)
      applyErrorEffects(err, { setOnline: ui.setOnline, markNeedsReauth: accounts.markNeedsReauth })
      ui.pushAlert(`${msg.discardFailed}${err.message ? `（${err.message}）` : ''}`)
      return false
    }
    items.value = items.value.filter(
      (x) => !(x.accountId === d.accountId && x.draftId === d.draftId),
    )
    ui.pushToast('success', msg.discarded)
    const open = inbox.selectedRef
    if (open && open.accountId === d.accountId && open.threadId === d.threadId) void thread.reload() // 開いているスレッドの「下書きを開く」を消す
    return true
  }

  // 作成セッションを閉じた・送信した・破棄したら、一覧を読み直す（保存のたびには読み直さない。1 回の読み込みが Gmail を何度も呼ぶため）。
  watch(
    () => compose.session?.sessionId,
    (now, prev) => {
      if (active.value && prev !== undefined && now === undefined) void load()
    },
  )
  // 絞り込みのアカウントが変わったら、読み直す。
  watch(
    () => inbox.filter.accountIds.join(','),
    () => {
      if (active.value) void load()
    },
  )

  return { active, items, loading, truncated, load, show, hide, open, isOpen, discard }
})
