import '../../../test/domPurifyShim'
import { describe, expect, test } from 'bun:test'
import { mount } from '@vue/test-utils'
import type {
  ThreadDetail,
  ThreadMessage,
} from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { FALLBACK_HEIGHT } from './bodyFrame'
import MessageBody from './MessageBody.vue'
import MessageCard from './MessageCard.vue'
import ReplyBar from './ReplyBar.vue'
import { defaultExpanded, nextExpanded } from './threadView'

const HTML =
  '<p>こんにちは</p><img src="https://img.example/a.png" alt="r"><img src="data:image/png;base64,AAAA" alt="i">'

function srcdocOf(w: ReturnType<typeof mount>): string {
  return w.get('[data-testid="body-frame"]').attributes('srcdoc') ?? ''
}

describe('MessageBody', () => {
  test('iframe の sandbox は allow-same-origin のみ', () => {
    const w = mount(MessageBody, { props: { html: HTML, hasRemoteImages: true } })
    const sandbox = w.get('iframe').attributes('sandbox')
    expect(sandbox).toBe('allow-same-origin')
    for (const bad of ['allow-scripts', 'allow-forms', 'allow-popups', 'allow-top-navigation']) {
      expect(sandbox).not.toContain(bad)
    }
  })

  test('既定ではリモート画像をブロックし、CSP は https: なし。ボタンは hasRemoteImages のときだけ', () => {
    const w = mount(MessageBody, { props: { html: HTML, hasRemoteImages: true } })
    const doc = srcdocOf(w)
    expect(doc).not.toContain('img.example')
    expect(doc).toContain('data:image/png;base64,AAAA')
    expect(doc).toContain('img-src data:;')
    expect(w.find('[data-testid="show-images"]').exists()).toBe(true)

    const none = mount(MessageBody, { props: { html: '<p>x</p>', hasRemoteImages: false } })
    expect(none.find('[data-testid="show-images"]').exists()).toBe(false)
  })

  test('「画像を表示」で再描画し、画像と https: の CSP が復元される。ボタンは消える', async () => {
    const w = mount(MessageBody, { props: { html: HTML, hasRemoteImages: true } })
    await w.get('[data-testid="show-images"]').trigger('click')
    const doc = srcdocOf(w)
    expect(doc).toContain('https://img.example/a.png')
    expect(doc).toContain('img-src data: https:;')
    expect(w.find('[data-testid="show-images"]').exists()).toBe(false)
  })

  test('画像の許可はメッセージ単位（別インスタンスには及ばない）', async () => {
    const a = mount(MessageBody, { props: { html: HTML, hasRemoteImages: true } })
    const b = mount(MessageBody, { props: { html: HTML, hasRemoteImages: true } })
    await a.get('[data-testid="show-images"]').trigger('click')
    expect(srcdocOf(a)).toContain('img.example')
    expect(srcdocOf(b)).not.toContain('img.example')
  })

  test('HTML が無ければ bodyText を pre で表示し、URL を自動リンク化せず、iframe を使わない', () => {
    const w = mount(MessageBody, {
      props: { text: '見て https://example.com <b>x</b>', hasRemoteImages: false },
    })
    expect(w.find('iframe').exists()).toBe(false)
    const pre = w.get('pre')
    expect(pre.text()).toBe('見て https://example.com <b>x</b>')
    expect(pre.find('a').exists()).toBe(false)
    expect(pre.find('b').exists()).toBe(false)
  })

  test('スクリプトを含む HTML は srcdoc に残らない', () => {
    const w = mount(MessageBody, {
      props: {
        html: '<p onclick="x()">a</p><script>alert(1)</script><iframe src="https://e.example"></iframe>',
        hasRemoteImages: false,
      },
    })
    const doc = srcdocOf(w)
    expect(doc).not.toContain('<script>alert')
    expect(doc).not.toContain('onclick')
    expect(doc).not.toContain('<iframe')
  })

  test('計測できない間（未ロード）は固定高をフォールバックとして使う', () => {
    const w = mount(MessageBody, { props: { html: '<p>x</p>', hasRemoteImages: false } })
    expect(w.get('iframe').attributes('style')).toContain(`height: ${FALLBACK_HEIGHT}px`)
  })
})

function message(id: string, isUnread: boolean, extra: Partial<ThreadMessage> = {}): ThreadMessage {
  return {
    id,
    from: { name: '山田', email: 'yamada@example.com' },
    to: [{ email: 'me@example.com' }],
    cc: [{ name: '佐藤', email: 'sato@example.com' }],
    date: Date.UTC(2020, 0, 2, 3, 4),
    isUnread,
    bodyText: `本文-${id}`,
    hasRemoteImages: false,
    attachments: null,
    ...extra,
  }
}

describe('MessageCard', () => {
  test('展開時は差出人・宛先・Cc・日時と本文を表示し、折りたたみ時は本文を出さない', () => {
    const open = mount(MessageCard, { props: { message: message('m1', false), expanded: true } })
    const text = open.text()
    expect(text).toContain('山田 <yamada@example.com>')
    expect(text).toContain('me@example.com')
    expect(text).toContain('佐藤 <sato@example.com>')
    expect(text).toContain('2020年1月')
    expect(text).toContain('本文-m1')

    const closed = mount(MessageCard, { props: { message: message('m1', false), expanded: false } })
    expect(closed.find('[data-testid="card-content"]').exists()).toBe(false)
    expect(closed.text()).not.toContain('本文-m1')
    expect(closed.text()).toContain('山田')
  })

  test('Cc が無ければ Cc 行を出さない', () => {
    const w = mount(MessageCard, {
      props: { message: message('m1', false, { cc: [] }), expanded: true },
    })
    expect(w.text()).not.toContain('Cc')
  })

  test('ヘッダのクリックで開閉でき、aria-expanded が追従する', async () => {
    const w = mount(MessageCard, { props: { message: message('m1', false), expanded: false } })
    const toggle = w.get('[data-testid="card-toggle"]')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    await toggle.trigger('click')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    expect(w.text()).toContain('本文-m1')
    await toggle.trigger('click')
    expect(w.find('[data-testid="card-content"]').exists()).toBe(false)
  })

  test('添付があれば名前・サイズ・保存ボタンを出し、保存で save を emit する', async () => {
    const att = { id: 'a1', filename: '請求書.pdf', mimeType: 'application/pdf', size: 2048 }
    const w = mount(MessageCard, {
      props: { message: message('m1', false, { attachments: [att] }), expanded: true },
    })
    expect(w.text()).toContain('請求書.pdf')
    expect(w.text()).toContain('2.0 KB')
    await w.get('[data-testid="attachment"] button').trigger('click')
    expect(w.emitted('saveAttachment')?.[0]).toEqual([att])
  })
})

describe('展開規則（threadView）', () => {
  test('最新と未読は展開、それ以外は折りたたみ', () => {
    const ids = defaultExpanded([
      message('a', false),
      message('b', true),
      message('c', false),
      message('d', false),
    ])
    expect([...ids].sort()).toEqual(['b', 'd'])
  })

  function detail(id: string, msgs: ThreadMessage[]): ThreadDetail {
    return {
      account: { id: 'a1' } as ThreadDetail['account'],
      threadId: id,
      subject: 's',
      messages: msgs,
      labels: [],
    }
  }

  test('別スレッドでは規則どおりに作り直す', () => {
    const next = nextExpanded(
      detail('t1', [message('a', false)]),
      detail('t2', [message('x', false), message('y', false)]),
      new Set(['a']),
    )
    expect([...next]).toEqual(['y'])
  })

  test('同じスレッドの再取得では開閉を保ち、新着だけに規則を適用する（既読化で閉じない）', () => {
    const prev = detail('t1', [message('a', true), message('b', false)])
    const cur = detail('t1', [message('a', false), message('b', false), message('c', false)])
    const next = nextExpanded(prev, cur, new Set(['a', 'b']))
    expect([...next].sort()).toEqual(['a', 'b', 'c'])
  })

  test('ユーザーが開いたカードは再取得後も開いたまま、閉じたままのカードは閉じたまま', () => {
    const msgs = [message('a', false), message('b', false), message('c', false)]
    const next = nextExpanded(detail('t1', msgs), detail('t1', msgs), new Set(['a', 'c']))
    expect([...next].sort()).toEqual(['a', 'c'])
  })

  test('スレッドが無ければ空', () => {
    expect(nextExpanded(undefined, undefined, new Set(['a'])).size).toBe(0)
  })
})

describe('ReplyBar', () => {
  test('既定は「全員に返信」で reply_all を emit する', async () => {
    const w = mount(ReplyBar)
    expect(w.get('[data-testid="reply-all"]').text()).toBe('全員に返信')
    await w.get('[data-testid="reply-all"]').trigger('click')
    expect(w.emitted('open')?.[0]).toEqual(['reply_all'])
  })

  test('下書きがあるときだけ「下書きを開く」を出す', async () => {
    expect(mount(ReplyBar).find('[data-testid="open-draft"]').exists()).toBe(false)
    const w = mount(ReplyBar, { props: { draftId: 'd1' } })
    await w.get('[data-testid="open-draft"]').trigger('click')
    expect(w.emitted('openDraft')).toHaveLength(1)
  })

  test('メニュートリガーに aria-label がある', () => {
    expect(mount(ReplyBar).get('[data-testid="reply-menu"]').attributes('aria-label')).toBe(
      '返信の種類',
    )
  })
})
