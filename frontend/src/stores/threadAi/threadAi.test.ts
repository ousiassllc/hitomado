import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { createPinia, setActivePinia } from 'pinia'
import {
  aiCallsOf,
  aiCtl,
  installAiMocks,
  resetAiState,
  restoreAiMocks,
  result,
} from '../../components/thread/ai/threadAiKit'
import { appError, REF, until } from '../../components/thread/threadTestKit'

installAiMocks()
afterAll(restoreAiMocks)

const { useThreadStore } = await import('../thread')
const { useThreadAiStore } = await import('./index')

const OTHER = { accountId: 'a1', threadId: 't2' }

beforeEach(() => {
  setActivePinia(createPinia())
  resetAiState()
  localStorage.clear()
})

function setup() {
  const thread = useThreadStore()
  thread.currentRef = { ...REF }
  return { thread, ai: useThreadAiStore() }
}

/** 解決するまで待つ Promise（生成の途中の状態を作る）。 */
function deferred<T>() {
  let resolve!: (v: T) => void
  const promise = new Promise<T>((r) => {
    resolve = r
  })
  return { promise, resolve }
}

describe('threadAiStore: 要約（F-17）', () => {
  test('summarize を呼んだときだけ SummarizeThread を呼び、結果を持つ', async () => {
    const { ai } = setup()
    expect(aiCallsOf('SummarizeThread')).toHaveLength(0) // 作っただけでは呼ばない
    await ai.summarize(REF)
    const calls = aiCallsOf('SummarizeThread')
    expect(calls).toHaveLength(1)
    expect(calls[0].args[0]).toEqual(REF)
    expect(typeof calls[0].args[1]).toBe('string') // requestId
    expect(ai.summary?.text).toBe('- 要約')
    expect(ai.status).toBe('idle')
    expect(ai.threadKey).toBe('a1:t1')
  })

  test('再生成で置き換わり、失敗しても直前の要約は消えない', async () => {
    const { ai } = setup()
    await ai.summarize(REF)
    aiCtl.responses.SummarizeThread = result('- 新しい要約', 3, 5)
    await ai.summarize(REF)
    expect(ai.summary?.text).toBe('- 新しい要約')
    expect(ai.lastCounts).toEqual({ included: 3, total: 5 })

    aiCtl.failures.SummarizeThread = appError('AI_FAILED', 'AI の応答が時間内に返りませんでした。')
    await ai.summarize(REF)
    expect(ai.summary?.text).toBe('- 新しい要約')
    expect(ai.status).toBe('error')
    expect(ai.error).toBe('AI の応答が時間内に返りませんでした。')
  })

  test('中止すると CancelGenerate を requestId つきで呼び、完了しても結果を捨てて idle に戻る', async () => {
    const { ai } = setup()
    const d = deferred<ReturnType<typeof result>>()
    aiCtl.responses.SummarizeThread = () => d.promise
    const p = ai.summarize(REF)
    await until(() => ai.status === 'summarizing')
    ai.cancel()
    expect(ai.status).toBe('idle')
    const id = aiCallsOf('SummarizeThread')[0].args[1]
    expect(aiCallsOf('CancelGenerate')[0].args).toEqual([id])
    d.resolve(result('- 遅れて届いた要約'))
    await p
    expect(ai.summary).toBeUndefined()
    expect(ai.error).toBeUndefined()
  })

  test('Go が CANCELLED で終わったときはエラーにしない', async () => {
    const { ai } = setup()
    aiCtl.failures.SummarizeThread = appError('CANCELLED', 'AI の生成を中止しました。')
    await ai.summarize(REF)
    expect(ai.status).toBe('idle')
    expect(ai.error).toBeUndefined()
  })

  test('要約と質問は同時に 1 つだけ（生成中はもう一方を始めない）', async () => {
    const { ai } = setup()
    const d = deferred<ReturnType<typeof result>>()
    aiCtl.responses.SummarizeThread = () => d.promise
    const p = ai.summarize(REF)
    await until(() => ai.busy)
    await ai.ask(REF, '期限は？')
    await ai.summarize(REF)
    expect(aiCallsOf('AskAboutThread')).toHaveLength(0)
    expect(aiCallsOf('SummarizeThread')).toHaveLength(1)
    d.resolve(result('- 要約'))
    await p
  })
})

describe('threadAiStore: 質問（F-18）', () => {
  test('成功すると質問と回答が会話に加わり、書きかけが空になる', async () => {
    const { ai } = setup()
    ai.draft = '  期限は？ '
    await ai.ask(REF, ai.draft)
    const c = aiCallsOf('AskAboutThread')[0]
    expect(c.args.slice(0, 1)).toEqual([REF])
    expect(c.args[2]).toBe('期限は？') // 前後の空白は除く
    expect(c.args[3]).toEqual([]) // 初回は history なし
    expect(ai.turns).toEqual([
      { role: 'user', text: '期限は？' },
      { role: 'assistant', text: '回答' },
    ])
    expect(ai.draft).toBe('')
  })

  test('追い質問では前の質問と回答が history に入る', async () => {
    const { ai } = setup()
    await ai.ask(REF, '1 つ目')
    await ai.ask(REF, '2 つ目')
    expect(aiCallsOf('AskAboutThread')[1].args[3]).toEqual([
      { role: 'user', text: '1 つ目' },
      { role: 'assistant', text: '回答' },
    ])
  })

  test('11 往復目以降は、古い往復を history から外し、その旨を知らせる', async () => {
    const { ai } = setup()
    for (let i = 1; i <= 10; i++) await ai.ask(REF, `q${i}`)
    expect(ai.droppedFromContext).toBe(false)
    expect((aiCallsOf('AskAboutThread')[9].args[3] as unknown[]).length).toBe(18)

    await ai.ask(REF, 'q11') // 11 往復目: 直前の 10 往復（20 件）まで入る
    const h11 = aiCallsOf('AskAboutThread')[10].args[3] as { text: string }[]
    expect(h11).toHaveLength(20)
    expect(h11[0].text).toBe('q1')
    expect(ai.droppedFromContext).toBe(true) // 次からは q1 が外れる

    await ai.ask(REF, 'q12')
    const h12 = aiCallsOf('AskAboutThread')[11].args[3] as { text: string }[]
    expect(h12).toHaveLength(20)
    expect(h12[0].text).toBe('q2') // 古い往復が外れた
    expect(ai.turns).toHaveLength(24) // 画面の会話は消さない
  })

  test('失敗・中止では質問が会話に加わらず、書きかけは入力欄に残る。それまでの会話・要約も消えない', async () => {
    const { ai } = setup()
    await ai.summarize(REF)
    await ai.ask(REF, '1 つ目')
    ai.draft = '2 つ目'

    aiCtl.failures.AskAboutThread = appError('RATE_LIMITED', '混み合っています。')
    await ai.ask(REF, ai.draft)
    expect(ai.status).toBe('error')
    expect(ai.error).toBe('混み合っています。')
    expect(ai.turns).toHaveLength(2)
    expect(ai.draft).toBe('2 つ目')
    expect(ai.summary?.text).toBe('- 要約')

    const d = deferred<ReturnType<typeof result>>()
    aiCtl.failures = {}
    aiCtl.responses.AskAboutThread = () => d.promise
    const p = ai.ask(REF, ai.draft)
    await until(() => ai.status === 'asking')
    ai.cancel()
    d.resolve(result('遅れた回答'))
    await p
    expect(ai.turns).toHaveLength(2)
    expect(ai.draft).toBe('2 つ目')
    expect(ai.status).toBe('idle')
  })

  test('空・空白だけの質問は送らない', async () => {
    const { ai } = setup()
    await ai.ask(REF, ' \n ')
    expect(aiCallsOf('AskAboutThread')).toHaveLength(0)
  })

  test('「新しい会話」は会話だけを消し、要約は残す', async () => {
    const { ai } = setup()
    await ai.summarize(REF)
    await ai.ask(REF, 'q')
    ai.newConversation()
    expect(ai.turns).toEqual([])
    expect(ai.droppedFromContext).toBe(false)
    expect(ai.summary?.text).toBe('- 要約')
  })
})

describe('threadAiStore: 破棄（スレッドの切り替え・閉じる・除かれる）', () => {
  async function filled() {
    const s = setup()
    await s.ai.summarize(REF)
    await s.ai.ask(REF, 'q')
    s.ai.draft = '書きかけ'
    return s
  }

  test('別のスレッドを開くと、要約・会話・書きかけ・エラーがすべて消える', async () => {
    const { thread, ai } = await filled()
    thread.currentRef = { ...OTHER }
    expect(ai.summary).toBeUndefined()
    expect(ai.turns).toEqual([])
    expect(ai.draft).toBe('')
    expect(ai.status).toBe('idle')
    expect(ai.threadKey).toBeUndefined()
  })

  test('閉じる・一覧から除かれる（currentRef が無くなる）でも消える', async () => {
    const { thread, ai } = await filled()
    thread.currentRef = undefined
    expect(ai.summary).toBeUndefined()
    expect(ai.turns).toEqual([])
    expect(ai.draft).toBe('')
  })

  test('同じスレッドの再取得（同期による更新）では消えない', async () => {
    const { thread, ai } = await filled()
    thread.currentRef = { ...REF } // 同じキーの別オブジェクト
    expect(ai.summary?.text).toBe('- 要約')
    expect(ai.turns).toHaveLength(2)
    expect(ai.draft).toBe('書きかけ')
  })

  test('進行中の生成は中止され、あとから結果が届いても前のスレッドの内容は出ない', async () => {
    const { thread, ai } = setup()
    const d = deferred<ReturnType<typeof result>>()
    aiCtl.responses.AskAboutThread = () => d.promise
    const p = ai.ask(REF, '前のスレッドの質問')
    await until(() => ai.status === 'asking')
    thread.currentRef = { ...OTHER }
    expect(aiCallsOf('CancelGenerate')).toHaveLength(1)
    expect(aiCallsOf('CancelGenerate')[0].args[0]).toBe(aiCallsOf('AskAboutThread')[0].args[1]) // 同じ requestId
    expect(ai.status).toBe('idle')
    d.resolve(result('前のスレッドの回答'))
    await p
    expect(ai.turns).toEqual([])
    expect(ai.draft).toBe('')
  })

  test('localStorage に何も残さない', async () => {
    await filled()
    expect(localStorage.length).toBe(0)
  })
})
