import { afterAll, beforeEach, describe, expect, test } from 'bun:test'
import { createPinia, setActivePinia } from 'pinia'
import {
  appError,
  callsOf,
  ctl,
  detail,
  installMocks,
  REF,
  resetState,
  restoreMocks,
} from './threadTestKit'

installMocks()
afterAll(restoreMocks)

const { useThreadStore } = await import('../../stores/thread')
const { useUiStore } = await import('../../stores/ui')

beforeEach(() => {
  setActivePinia(createPinia())
  resetState()
})

describe('threadStore: 操作', () => {
  test('toggleRead は現在の状態の逆にし、成功で unread を反転する', async () => {
    ctl.responses.GetThread = detail(false)
    const s = useThreadStore()
    await s.open(REF)
    expect(s.unread).toBe(false)
    expect(await s.toggleRead()).toBe(true)
    expect(callsOf('MarkRead').at(-1)?.args).toEqual([[REF], false])
    expect(s.unread).toBe(true)
    expect(await s.toggleRead()).toBe(true)
    expect(callsOf('MarkRead').at(-1)?.args).toEqual([[REF], true])
    expect(s.unread).toBe(false)
  })

  test('archive／trash は inboxStore 経由で現在のスレッドに対して呼ぶ', async () => {
    ctl.responses.GetThread = detail(false)
    ctl.responses.Archive = []
    ctl.responses.Trash = []
    const s = useThreadStore()
    await s.open(REF)
    await s.archive()
    await s.trash()
    expect(callsOf('Archive')[0].args).toEqual([[REF]])
    expect(callsOf('Trash')[0].args).toEqual([[REF]])
  })

  test('openExternal は許可スキームだけ OpenExternal を呼ぶ', async () => {
    const s = useThreadStore()
    await s.openExternal('javascript:alert(1)')
    await s.openExternal('file:///etc/passwd')
    expect(callsOf('OpenExternal')).toHaveLength(0)
    await s.openExternal('https://example.com/')
    await s.openExternal('mailto:a@example.com')
    expect(callsOf('OpenExternal').map((c) => c.args[0])).toEqual([
      'https://example.com/',
      'mailto:a@example.com',
    ])
  })

  test('saveAttachment: 保存で成功トースト、キャンセル（saved=false）は何も出さず、失敗はエラーアラート', async () => {
    const s = useThreadStore()
    const ui = useUiStore()
    await s.open(REF)
    const att = { id: 'att1', filename: 'a.pdf', mimeType: 'application/pdf', size: 3 }
    ctl.responses.SaveAttachment = { saved: false }
    await s.saveAttachment('m1', att)
    expect(ui.toast).toBeUndefined()
    expect(ui.alerts).toHaveLength(0)
    expect(callsOf('SaveAttachment')[0].args).toEqual([
      { ...REF, messageId: 'm1' },
      'att1',
      'a.pdf',
    ])
    ctl.responses.SaveAttachment = { saved: true, path: '/x/a.pdf' }
    await s.saveAttachment('m1', att)
    expect(ui.toast?.kind).toBe('success')
    ctl.failures.SaveAttachment = appError('ATTACHMENT_IO', '書き込めません')
    await s.saveAttachment('m1', att)
    expect(ui.alerts.at(-1)?.kind).toBe('error')
  })

  test('ラベル: loadLabels はアカウントごとに 1 度だけ取得し、toggleLabel は付与・除去を切り替える', async () => {
    const s = useThreadStore()
    await s.open(REF)
    await s.loadLabels()
    await s.loadLabels()
    expect(callsOf('ListLabels')).toHaveLength(1)
    expect(s.accountLabels.map((l) => l.id)).toEqual(['L1'])

    await s.toggleLabel('L1')
    expect(callsOf('ModifyLabels')[0].args).toEqual([REF, ['L1'], []])
    ctl.responses.GetThread = detail(false, { labels: [{ id: 'L1', name: '仕事' }] })
    await s.reload()
    await s.toggleLabel('L1')
    expect(callsOf('ModifyLabels')[1].args).toEqual([REF, [], ['L1']])
  })
})
