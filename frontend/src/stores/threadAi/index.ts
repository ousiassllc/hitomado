import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'
import { AIService } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app'
import type {
  ThreadAIResult,
  ThreadRef,
} from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { call, parseBindingError } from '../../lib/api'
import { threadKey } from '../../lib/inboxItems'
import { type ThreadAiTurn, trimHistory } from '../../lib/threadAi'
import { useThreadStore } from '../thread'

/** 生成の状態。要約と質問は同時に 1 つだけ（summarizing／asking の間は、どちらも始められない）。 */
export type ThreadAiStatus = 'idle' | 'summarizing' | 'asking' | 'error'

/**
 * スレッドの AI 要約・質問（F-17・F-18）の状態。**すべてメモリのみ**（`localStorage`・設定・ログに保存しない）。
 * 開いているスレッドが変わる・閉じる・一覧から除かれるで破棄し、進行中の生成は中止する（同じスレッドの再取得では破棄しない）。
 * 前提確認（設定・キー・同意）は呼び出し側（useThreadAi）が済ませてから summarize／ask を呼ぶ。自動では呼ばない。
 */
export const useThreadAiStore = defineStore('threadAi', () => {
  const thread = useThreadStore()

  const summary = ref<ThreadAIResult>()
  /** 会話の全体（質問と回答が交互。画面に出す）。生成に成功した質問だけが加わる。 */
  const turns = ref<ThreadAiTurn[]>([])
  /** 文脈（最新 10 往復・20,000 文字）から外した往復があるか。 */
  const droppedFromContext = ref(false)
  /** 質問の書きかけ。 */
  const draft = ref('')
  const status = ref<ThreadAiStatus>('idle')
  const error = ref<string>()
  /** どのスレッドの結果か（`accountId:threadId`）。 */
  const key = ref<string>()
  /** 最後の回答の `includedCount`／`totalCount`（長いスレッドで古いメッセージを外したかの表示用）。要約が無ければ回答のもの。 */
  const lastCounts = ref<{ included: number; total: number }>()

  const busy = computed(() => status.value === 'summarizing' || status.value === 'asking')

  /** 実行中の生成の識別子（0 は無し）。中止・破棄で 0 に戻すので、古い生成の完了・失敗が来ても結果を捨てられる。 */
  let currentRun = 0
  let runSeq = 0
  let requestId: string | undefined

  function discardInFlight(): void {
    const id = requestId
    if (id !== undefined) void call(AIService.CancelGenerate(id)).catch(() => undefined)
    requestId = undefined
  }

  /** すべて破棄し、進行中の生成を中止する。 */
  function reset(): void {
    currentRun = 0
    discardInFlight()
    summary.value = undefined
    turns.value = []
    droppedFromContext.value = false
    draft.value = ''
    status.value = 'idle'
    error.value = undefined
    key.value = undefined
    lastCounts.value = undefined
  }

  // 開いているスレッドが変わる・閉じる・一覧から除かれる（= currentRef が変わる）で破棄する。同期による再取得では currentRef は変わらない。
  watch(
    () =>
      thread.currentRef
        ? threadKey(thread.currentRef.accountId, thread.currentRef.threadId)
        : undefined,
    () => reset(),
    { flush: 'sync' },
  )

  /** 1 回の生成を実行する。成功したら onResult に渡す。中止は何も変えず、失敗は error に入れる（要約・会話は消さない）。 */
  async function run(
    ref: ThreadRef,
    mode: 'summarizing' | 'asking',
    send: (id: string) => Promise<ThreadAIResult>,
    onResult: (r: ThreadAIResult) => void,
  ): Promise<void> {
    if (busy.value) return
    const run = ++runSeq
    currentRun = run
    const id = crypto.randomUUID()
    requestId = id
    key.value = threadKey(ref.accountId, ref.threadId)
    status.value = mode
    error.value = undefined
    try {
      const r = await send(id)
      if (run !== currentRun) return
      onResult(r)
      status.value = 'idle'
    } catch (e) {
      if (run !== currentRun) return
      const err = parseBindingError(e)
      if (err.code === 'CANCELLED') {
        status.value = 'idle'
        return
      }
      error.value = err.message
      status.value = 'error'
    } finally {
      if (run === currentRun) {
        currentRun = 0
        requestId = undefined
      }
    }
  }

  /** 要約を生成する（「AI で要約」ボタンからのみ）。完了したら要約を置き換える。失敗しても直前の要約は残す。 */
  async function summarize(ref: ThreadRef): Promise<void> {
    await run(
      ref,
      'summarizing',
      (id) => call(AIService.SummarizeThread(ref, id)),
      (r) => {
        summary.value = r
        lastCounts.value = { included: r.includedCount, total: r.totalCount }
      },
    )
  }

  /**
   * 質問を送る（質問の送信操作からのみ）。会話は最新 10 往復・20,000 文字に収めた history として渡す。
   * 成功したら質問と回答を会話に加えて書きかけを空にする。中止・失敗では会話に加えず、書きかけも残す（入力欄に戻る）。
   */
  async function ask(ref: ThreadRef, question: string): Promise<void> {
    const q = question.trim()
    if (q === '') return
    const { history } = trimHistory(turns.value)
    await run(
      ref,
      'asking',
      (id) => call(AIService.AskAboutThread(ref, id, q, history)),
      (r) => {
        turns.value = [
          ...turns.value,
          { role: 'user', text: q },
          { role: 'assistant', text: r.text },
        ]
        droppedFromContext.value = trimHistory(turns.value).dropped
        lastCounts.value = { included: r.includedCount, total: r.totalCount }
        draft.value = ''
      },
    )
  }

  /** 生成中の中止。画面はすぐ戻し、結果が来ても捨てる。 */
  function cancel(): void {
    if (!busy.value) return
    currentRun = 0
    discardInFlight()
    status.value = 'idle'
  }

  /** 「新しい会話」。会話だけを消す（要約は残す）。 */
  function newConversation(): void {
    if (status.value === 'asking') return
    turns.value = []
    droppedFromContext.value = false
    error.value = undefined
  }

  return {
    summary,
    turns,
    droppedFromContext,
    draft,
    status,
    error,
    threadKey: key,
    lastCounts,
    busy,
    summarize,
    ask,
    cancel,
    newConversation,
    reset,
  }
})
