import { afterAll, afterEach, beforeEach, describe, expect, mock, test } from 'bun:test'
import { mount, type VueWrapper } from '@vue/test-utils'
import { handlers, installMocks, resetKit, restoreMocks } from '../inboxKit'

installMocks()
afterAll(restoreMocks)

const { default: AttachmentBar } = await import('../../src/components/compose/AttachmentBar.vue')
const { useComposeStore } = await import('../../src/stores/compose')

// 失敗したテストのリスナーが後続へ残らないよう、テストごとに必ず外す
const mounted: VueWrapper[] = []
afterEach(() => {
  for (const w of mounted.splice(0)) w.unmount()
})
beforeEach(resetKit)

/** 添付の API（addAttachments）を差し替えたストアを返す。 */
function setup() {
  const compose = useComposeStore()
  const add = mock(async (_paths: string[]) => true)
  compose.addAttachments = add
  mounted.push(mount(AttachmentBar))
  return { compose, add }
}

const drop = (data: unknown) => handlers.get('files-dropped')?.(data)

describe('AttachmentBar の files-dropped', () => {
  test('ドロップされたパスで添付を呼ぶ（単体の形・配列に包まれた形の両方）', () => {
    const { add } = setup()
    drop({ files: ['/tmp/a.txt', '/tmp/b.txt'] })
    drop([{ files: ['/tmp/c.txt'] }])
    expect(add.mock.calls).toEqual([[['/tmp/a.txt', '/tmp/b.txt']], [['/tmp/c.txt']]])
  })

  test('パスが取り出せないペイロードでは添付を呼ばない', () => {
    const { add } = setup()
    drop({ files: [] })
    drop(null)
    drop('x')
    expect(add).not.toHaveBeenCalled()
  })

  test('送信中・送信確認中は添付しない', () => {
    const { compose, add } = setup()
    compose.sending = true
    drop({ files: ['/tmp/a.txt'] })
    compose.sending = false
    compose.preview = { sessionId: 's' } as never
    drop({ files: ['/tmp/a.txt'] })
    expect(add).not.toHaveBeenCalled()
  })

  test('アンマウントで購読を解除する', () => {
    const { add } = setup()
    expect(handlers.has('files-dropped')).toBe(true)
    mounted.pop()?.unmount()
    expect(handlers.has('files-dropped')).toBe(false)
    drop({ files: ['/tmp/a.txt'] })
    expect(add).not.toHaveBeenCalled()
  })
})
