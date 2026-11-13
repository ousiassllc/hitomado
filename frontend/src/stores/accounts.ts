import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import * as AccountService from '../../bindings/github.com/ousiassllc/hitomado/internal/app/accountservice'
import type {
  Account,
  AccountPatch,
  GmailSignature,
  OAuthClient,
} from '../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { call, list, parseBindingError } from '../lib/api'
import { type AccountStatus, type AccountStatusPayload, EVENTS, onEvent } from '../lib/events'
import { common } from '../lib/messages'
import { makeApi } from './apiCall'
import { useUiStore } from './ui'

const FALLBACK_COLOR = 'blue'

function normalizeStatus(s: string): AccountStatus {
  return s === 'needs_reauth' || s === 'error' || s === 'syncing' ? s : 'active'
}

export const useAccountsStore = defineStore('accounts', () => {
  const ui = useUiStore()
  const accounts = ref<Account[]>([])
  const oauthClients = ref<OAuthClient[]>([])
  const statuses = ref<Record<string, AccountStatus>>({})
  const statusMessages = ref<Record<string, string>>({})
  let started = false
  let stopEvents: (() => void) | undefined

  const byId = computed(() => {
    const m = new Map(accounts.value.map((a) => [a.id, a]))
    return (id: string): Account | undefined => m.get(id)
  })

  function colorOf(id: string): string {
    return byId.value(id)?.color || FALLBACK_COLOR
  }

  function statusOf(id: string): AccountStatus {
    return statuses.value[id] ?? 'active'
  }

  function markNeedsReauth(accountId: string): void {
    setStatus(accountId, 'needs_reauth')
  }

  function setStatus(accountId: string, status: AccountStatus, message?: string): void {
    statuses.value = { ...statuses.value, [accountId]: status }
    const next = { ...statusMessages.value }
    if (message) next[accountId] = message
    else delete next[accountId]
    statusMessages.value = next
  }

  const api = makeApi(ui, markNeedsReauth)

  function setAccounts(next: Account[]): void {
    accounts.value = next
    const st: Record<string, AccountStatus> = {}
    const msgs: Record<string, string> = {}
    for (const a of next) {
      st[a.id] = statuses.value[a.id] === 'syncing' ? 'syncing' : normalizeStatus(a.status) // 'syncing' は一時状態。DB 由来の値で上書きしない（理由も DB から取り込む: 再起動後は account:status が来ない）
      const m = st[a.id] === 'syncing' ? statusMessages.value[a.id] : a.statusMessage
      if (m) msgs[a.id] = m
    }
    statuses.value = st
    statusMessages.value = msgs
  }

  function upsert(a: Account): void {
    const exists = accounts.value.some((x) => x.id === a.id)
    setAccounts(
      exists ? accounts.value.map((x) => (x.id === a.id ? a : x)) : [...accounts.value, a],
    )
  }

  async function load(): Promise<void> {
    const [acc, clients] = await Promise.all([
      api(AccountService.ListAccounts()),
      api(AccountService.ListOAuthClients()),
    ])
    setAccounts(list(acc))
    oauthClients.value = list(clients)
  }

  async function loadOAuthClients(): Promise<void> {
    oauthClients.value = list(await api(AccountService.ListOAuthClients()))
  }

  async function add(oauthClientId: string): Promise<Account> {
    const a = await api(AccountService.StartAddAccount(oauthClientId))
    upsert(a)
    await loadOAuthClients().catch(() => undefined)
    return a
  }

  async function cancelAdd(): Promise<void> {
    try {
      await call(AccountService.CancelAddAccount())
    } catch (e) {
      ui.pushAlert(parseBindingError(e).message)
    }
  }

  async function reconnect(id: string): Promise<Account> {
    const a = await api(AccountService.Reconnect(id))
    upsert(a)
    setStatus(id, normalizeStatus(a.status))
    return a
  }

  async function remove(id: string): Promise<void> {
    await api(AccountService.RemoveAccount(id))
    setAccounts(accounts.value.filter((a) => a.id !== id))
    await loadOAuthClients().catch(() => undefined)
  }

  async function update(id: string, patch: AccountPatch): Promise<Account> {
    const a = await api(AccountService.UpdateAccount(id, patch))
    upsert(a)
    return a
  }

  async function setSignature(id: string, html: string): Promise<Account> {
    const a = await api(AccountService.SetSignature(id, html))
    upsert(a)
    return a
  }

  /** Gmail の署名を取得して返す（保存しない。保存は setSignature）。 */
  function fetchGmailSignature(id: string): Promise<GmailSignature> {
    return api(AccountService.FetchGmailSignature(id))
  }

  async function reorder(ids: string[]): Promise<void> {
    setAccounts(list(await api(AccountService.ReorderAccounts(ids))))
  }

  async function addOAuthClient(
    name: string,
    clientId: string,
    secret: string,
  ): Promise<OAuthClient> {
    const c = await api(AccountService.AddOAuthClient(name, clientId, secret))
    oauthClients.value = [...oauthClients.value, c]
    return c
  }

  async function updateOAuthClient(
    id: string,
    name: string,
    clientId: string,
    secret?: string,
  ): Promise<OAuthClient> {
    const c = await api(AccountService.UpdateOAuthClient(id, name, clientId, secret ?? ''))
    oauthClients.value = oauthClients.value.map((x) => (x.id === id ? c : x))
    return c
  }

  async function deleteOAuthClient(id: string): Promise<void> {
    await api(AccountService.DeleteOAuthClient(id))
    oauthClients.value = oauthClients.value.filter((x) => x.id !== id)
  }

  function onAccountStatus(p: AccountStatusPayload): void {
    setStatus(p.accountId, normalizeStatus(p.status), p.message)
    if (p.status !== 'syncing') {
      accounts.value = accounts.value.map((a) =>
        a.id === p.accountId ? { ...a, status: p.status, statusMessage: p.message } : a,
      )
    }
  }

  /**
   * 前回の起動が Google の認可の途中で止められていたら（Android がブラウザに出ている間にプロセスを終了させた場合）、
   * 待受が消えていて認可は完了できないため、やり直しを案内する。検出は Go が 1 回だけ返す。失敗しても何もしない。
   */
  async function notifyInterruptedAuth(): Promise<void> {
    try {
      if (await AccountService.TakeInterruptedAuth()) {
        ui.pushNotice(common.addAccount.interrupted)
      }
    } catch {
      // 案内できなくても通常の利用には影響しない。
    }
  }

  /** 読み込む。OFFLINE 以外の失敗は再試行つきのアラートで知らせる（OFFLINE は api() が共通のオフライン表示にする）。 */
  async function loadOrAlert(): Promise<void> {
    try {
      await load()
    } catch (e) {
      const err = parseBindingError(e)
      if (err.code !== 'OFFLINE') ui.pushAlert(err.message, () => void loadOrAlert())
    }
  }

  /** イベント購読と初期ロード（冪等）。 */
  async function start(): Promise<void> {
    if (started) return
    started = true
    stopEvents = onEvent<AccountStatusPayload>(EVENTS.ACCOUNT_STATUS, onAccountStatus)
    await loadOrAlert()
    await notifyInterruptedAuth()
  }

  function stop(): void {
    stopEvents?.()
    stopEvents = undefined
    started = false
  }

  return {
    accounts,
    oauthClients,
    statuses,
    statusMessages,
    byId,
    colorOf,
    statusOf,
    markNeedsReauth,
    load,
    loadOAuthClients,
    add,
    cancelAdd,
    reconnect,
    remove,
    update,
    setSignature,
    fetchGmailSignature,
    reorder,
    addOAuthClient,
    updateOAuthClient,
    deleteOAuthClient,
    onAccountStatus,
    start,
    stop,
  }
})
