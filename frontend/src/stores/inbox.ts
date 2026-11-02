import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import * as AppService from '../../bindings/github.com/ousiassllc/hitomado/internal/app/appservice'
import type {
  InboxFilter,
  ThreadPage,
  ThreadSummary,
} from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import * as InboxService from '../../bindings/github.com/ousiassllc/hitomado/internal/app/inboxservice'
import { type AppError, call, list, parseBindingError } from '../lib/api'
import { EVENTS, onEvent, type SyncUpdatedPayload } from '../lib/events'
import { keyOf, mergeHead } from '../lib/inboxItems'
import { common } from '../lib/messages'
import { useAccountsStore } from './accounts'
import { makeApi } from './apiCall'
import { type Failure, reportRefreshResults, toFailures } from './inboxFailures'
import { watchRemovedAccounts } from './inboxFilterSync'
import { useInboxMutations } from './inboxMutations'
import { useInboxSelection } from './inboxSelection'
import { useUiStore } from './ui'

const PAGE_SIZE = 50
/** 行が 0 件のページが続いても 1 回の読み込みで辿る最大ページ数 */
const MAX_EMPTY_PAGES = 10

export const useInboxStore = defineStore('inbox', () => {
  const ui = useUiStore()
  const accounts = useAccountsStore()

  const filter = ref<{ accountIds: string[]; unreadOnly: boolean }>({
    accountIds: [],
    unreadOnly: false,
  })
  const mode = ref<'inbox' | 'search'>('inbox')
  const searchQuery = ref('')
  const items = ref<ThreadSummary[]>([])
  const nextCursor = ref<string>()
  const unreadCounts = ref<Record<string, number>>({})
  const loading = ref(false)
  const loadingMore = ref(false)
  const refreshing = ref(false)
  const failures = ref<Failure[]>([])

  let started = false
  let generation = 0
  let countsSeq = 0
  let unsubscribers: Array<() => void> = []
  let reloading = false
  let reloadDirty = false
  /** サーバー反映待ちで楽観的に除去した行。先頭ページの再取得で復活させない。 */
  const pendingRemoval = new Set<string>()

  const selection = useInboxSelection(items)
  const hasMore = computed(() => nextCursor.value !== undefined && nextCursor.value !== '')

  const api = makeApi(ui, accounts.markNeedsReauth)

  const mutations = useInboxMutations({
    items,
    hidesRead: () => filter.value.unreadOnly && mode.value === 'inbox',
    pendingRemoval,
    api,
    ui,
    afterRemoval: selection.afterRemoval,
    reload: () => void reloadHead(),
  })

  function applyFailures(fs: Failure[]): void {
    failures.value = fs
    for (const f of fs) {
      if (f.code === 'NEEDS_REAUTH') accounts.markNeedsReauth(f.accountId)
      else if (f.code === 'OFFLINE') ui.setOnline(false)
    }
  }

  function fetchPage(cursor?: string): Promise<ThreadPage> {
    const base = { accountIds: [...filter.value.accountIds], limit: PAGE_SIZE, cursor }
    if (mode.value === 'search') return api(InboxService.Search({ q: searchQuery.value, ...base }))
    return api(InboxService.ListInbox({ unreadOnly: filter.value.unreadOnly, ...base }))
  }

  async function fetchUnreadCounts(): Promise<void> {
    const seq = ++countsSeq
    try {
      const c = await api(InboxService.GetUnreadCounts())
      if (seq !== countsSeq) return // より新しい取得が始まっている。古い応答で上書きしない
      unreadCounts.value = Object.fromEntries(Object.entries(c ?? {}).map(([k, v]) => [k, v ?? 0]))
    } catch {
      // 件数は補助表示。失敗は api() が共通状態に反映済み。
    }
  }

  /** 先頭から読み直す（フィルタ変更・検索・初回）。 */
  async function load(): Promise<void> {
    const gen = ++generation
    loading.value = true
    loadingMore.value = false // 進行中の loadMore は破棄される（その finally は世代違いで触らない）
    try {
      const page = await fetchPage()
      if (gen !== generation) return
      items.value = list(page.items).filter((t) => !pendingRemoval.has(keyOf(t)))
      nextCursor.value = page.nextCursor || undefined
      applyFailures(toFailures(page.failures))
      selection.pruneSelection()
    } catch (e) {
      if (gen !== generation) return
      const err = e as AppError
      if (err.code !== 'OFFLINE') {
        ui.pushToast(
          'error',
          mode.value === 'search' ? common.op.searchFailed : common.op.loadFailed,
          () => void load(),
        )
      }
    } finally {
      if (gen === generation) loading.value = false
    }
    if (gen === generation && items.value.length === 0 && hasMore.value) await loadMore()
    if (mode.value === 'inbox') await fetchUnreadCounts()
  }

  async function loadMore(): Promise<void> {
    if (loading.value || loadingMore.value || !hasMore.value) return
    const gen = generation
    loadingMore.value = true
    try {
      // 条件に合う行が 0 件でも Gmail 側に続きがあれば NextCursor が返る。追加できるまで（上限付きで）続ける。
      for (let i = 0; i < MAX_EMPTY_PAGES && hasMore.value; i++) {
        const page = await fetchPage(nextCursor.value)
        if (gen !== generation) return
        const have = new Set(items.value.map(keyOf))
        const more = list(page.items).filter(
          (t) => !have.has(keyOf(t)) && !pendingRemoval.has(keyOf(t)),
        )
        items.value = [...items.value, ...more]
        nextCursor.value = page.nextCursor || undefined
        applyFailures(toFailures(page.failures))
        // 追加 0 件で失敗を含むページは 1 回で打ち切る（失敗するアカウントへ連続して再要求しない）。
        if (more.length > 0 || list(page.failures).length > 0) break
      }
    } catch (e) {
      if ((e as AppError).code !== 'OFFLINE')
        ui.pushToast('error', common.op.loadFailed, () => void loadMore())
    } finally {
      if (gen === generation) loadingMore.value = false
    }
  }

  async function reloadHeadOnce(): Promise<void> {
    const gen = generation
    const page = await fetchPage()
    if (gen !== generation) return
    const merged = mergeHead(
      items.value,
      list(page.items),
      page.nextCursor !== undefined && page.nextCursor !== '',
      pendingRemoval,
    )
    items.value = merged.items
    if (!merged.keptTail) nextCursor.value = page.nextCursor || undefined
    applyFailures(toFailures(page.failures))
    selection.pruneSelection()
  }

  /** 先頭ページと未読件数を再取得する。スクロール位置・選択は維持する（検索結果は再取得しない）。 */
  async function reloadHead(): Promise<void> {
    if (reloading) {
      reloadDirty = true
      return
    }
    reloading = true
    try {
      do {
        reloadDirty = false
        if (mode.value === 'inbox') {
          try {
            await reloadHeadOnce()
          } catch {
            // 一覧は取得済みの内容を維持する。状態は api() が反映済み。
          }
        }
        await fetchUnreadCounts()
      } while (reloadDirty)
    } finally {
      reloading = false
    }
  }

  /** 手動更新: 差分同期を今すぐ実行して再取得する。 */
  async function refresh(): Promise<void> {
    if (refreshing.value) return
    refreshing.value = true
    try {
      const res = await api(InboxService.Refresh([...filter.value.accountIds]))
      reportRefreshResults(list(res.results), {
        markNeedsReauth: accounts.markNeedsReauth,
        setOffline: () => ui.setOnline(false),
        nameOf: (id) => accounts.byId(id)?.displayName ?? id,
        toast: (m) => ui.pushToast('error', m),
      })
    } catch (e) {
      if ((e as AppError).code !== 'OFFLINE')
        ui.pushToast('error', common.op.loadFailed, () => void refresh())
    } finally {
      refreshing.value = false
    }
    await load()
  }

  async function runSearch(q: string): Promise<void> {
    const query = q.trim()
    if (!query) return clearSearch()
    mode.value = 'search'
    searchQuery.value = query
    selection.clearSelection()
    await load()
  }

  async function clearSearch(): Promise<void> {
    if (mode.value === 'inbox') return
    mode.value = 'inbox'
    searchQuery.value = ''
    selection.clearSelection()
    await load()
  }

  async function setFilter(f: InboxFilter): Promise<void> {
    filter.value = { accountIds: list(f.accountIds), unreadOnly: f.unreadOnly }
    try {
      await call(
        AppService.SetInboxFilter({
          accountIds: filter.value.accountIds,
          unreadOnly: filter.value.unreadOnly,
        }),
      )
    } catch (e) {
      ui.pushToast('error', parseBindingError(e).message)
    }
    await load()
  }

  function onSyncUpdated(_p: SyncUpdatedPayload): void {
    if (!ui.online) ui.setOnline(true)
    void reloadHead()
  }

  /** 保存済みフィルタを復元し、イベントを購読して初回ロードする（冪等）。 */
  async function start(): Promise<void> {
    if (started) return
    started = true
    unsubscribers = [
      onEvent<SyncUpdatedPayload>(EVENTS.SYNC_UPDATED, onSyncUpdated),
      watchRemovedAccounts(accounts, filter, setFilter),
    ]
    try {
      const f = await api(AppService.GetInboxFilter())
      filter.value = { accountIds: list(f.accountIds), unreadOnly: f.unreadOnly === true }
    } catch {
      // 既定（すべて・未読のみ OFF）で続行する
    }
    await load()
  }

  function stop(): void {
    for (const off of unsubscribers) off()
    unsubscribers = []
    started = false
  }

  return {
    filter,
    mode,
    searchQuery,
    items,
    nextCursor,
    unreadCounts,
    loading,
    loadingMore,
    refreshing,
    failures,
    hasMore,
    ...selection,
    ...mutations,
    load,
    loadMore,
    refresh,
    runSearch,
    clearSearch,
    setFilter,
    onSyncUpdated,
    start,
    stop,
  }
})

export type { Failure }
