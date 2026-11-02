import { describe, expect, test } from 'bun:test'
import {
  hasOpenDialog,
  hasOpenPopup,
  isEditableElement,
  isInteractiveElement,
  type KeyInfo,
  resolveShortcut,
  SHORTCUT_KEYS,
  type ShortcutAction,
  type ShortcutContext,
} from '../src/lib/shortcuts'

function key(k: string, extra: Partial<KeyInfo> = {}): KeyInfo {
  return {
    key: k,
    shiftKey: false,
    ctrlKey: false,
    metaKey: false,
    altKey: false,
    repeat: false,
    inEditable: false,
    onInteractive: false,
    ...extra,
  }
}

const idle: ShortcutContext = { dialogOpen: false, pendingG: false }

describe('SHORTCUT_KEYS（ボタン併記のキー表記）', () => {
  test('表記のキーを押すと同じアクションに解決される', () => {
    for (const [action, label] of Object.entries(SHORTCUT_KEYS)) {
      const shift = label.startsWith('Shift+')
      const k = shift ? label.slice('Shift+'.length) : label
      expect(resolveShortcut(key(k, { shiftKey: shift }), idle).action).toBe(
        action as ShortcutAction,
      )
    }
  })
})

describe('resolveShortcut', () => {
  test('単独キーの割り当て', () => {
    const table: [string, ShortcutAction][] = [
      ['j', 'next'],
      ['k', 'prev'],
      ['o', 'open'],
      ['Enter', 'open'],
      ['Escape', 'escape'],
      ['e', 'archive'],
      ['r', 'reply'],
      ['a', 'replyAll'],
      ['f', 'forward'],
      ['c', 'compose'],
      ['/', 'search'],
    ]
    for (const [k, action] of table) expect(resolveShortcut(key(k), idle).action).toBe(action)
  })

  test('# と Shift+i / Shift+u', () => {
    expect(resolveShortcut(key('#', { shiftKey: true }), idle).action).toBe('trash')
    expect(resolveShortcut(key('#'), idle).action).toBe('trash')
    expect(resolveShortcut(key('I', { shiftKey: true }), idle).action).toBe('markRead')
    expect(resolveShortcut(key('U', { shiftKey: true }), idle).action).toBe('markUnread')
  })

  test('Shift 付きの j や e は無効（Gmail の別機能と衝突させない）', () => {
    expect(resolveShortcut(key('J', { shiftKey: true }), idle).action).toBeUndefined()
    expect(resolveShortcut(key('E', { shiftKey: true }), idle).action).toBeUndefined()
  })

  test('入力要素・ダイアログ表示中は無効', () => {
    for (const k of ['j', 'e', 'c', '/', 'Escape', 'Enter', '#']) {
      expect(resolveShortcut(key(k, { inEditable: true }), idle).action).toBeUndefined()
      expect(resolveShortcut(key(k), { dialogOpen: true, pendingG: false }).action).toBeUndefined()
    }
  })

  test('Ctrl / Meta / Alt 併用は無効', () => {
    expect(resolveShortcut(key('r', { ctrlKey: true }), idle).action).toBeUndefined()
    expect(resolveShortcut(key('c', { metaKey: true }), idle).action).toBeUndefined()
    expect(resolveShortcut(key('e', { altKey: true }), idle).action).toBeUndefined()
    expect(resolveShortcut(key('Enter', { ctrlKey: true }), idle).action).toBeUndefined()
  })

  test('ボタン上の Enter は奪わない（他のキーは有効）', () => {
    expect(resolveShortcut(key('Enter', { onInteractive: true }), idle).action).toBeUndefined()
    expect(resolveShortcut(key('j', { onInteractive: true }), idle).action).toBe('next')
  })

  test('g → i で受信箱へ（2 打鍵）', () => {
    const first = resolveShortcut(key('g'), idle)
    expect(first).toEqual({ action: undefined, pendingG: true })
    const second = resolveShortcut(key('i'), { dialogOpen: false, pendingG: first.pendingG })
    expect(second).toEqual({ action: 'goInbox', pendingG: false })
  })

  test('g の後に別のキーなら待ちを解除してそのキーを通常処理する', () => {
    const r = resolveShortcut(key('j'), { dialogOpen: false, pendingG: true })
    expect(r).toEqual({ action: 'next', pendingG: false })
  })

  test('g 待ちでない i は何もしない', () => {
    expect(resolveShortcut(key('i'), idle).action).toBeUndefined()
  })

  test('g 待ち中にダイアログが開けば無効になり待ちも解除される', () => {
    expect(resolveShortcut(key('i'), { dialogOpen: true, pendingG: true })).toEqual({
      pendingG: false,
    })
  })
})

describe('自動リピート', () => {
  test('操作系・作成系は無視し、j/k などの移動は許可する', () => {
    for (const k of ['e', '#', 'r', 'a', 'f', 'c']) {
      expect(resolveShortcut(key(k, { repeat: true }), idle)).toEqual({ pendingG: false })
    }
    expect(resolveShortcut(key('I', { shiftKey: true, repeat: true }), idle).action).toBeUndefined()
    expect(resolveShortcut(key('U', { shiftKey: true, repeat: true }), idle).action).toBeUndefined()
    expect(resolveShortcut(key('j', { repeat: true }), idle).action).toBe('next')
    expect(resolveShortcut(key('k', { repeat: true }), idle).action).toBe('prev')
    expect(resolveShortcut(key('Escape', { repeat: true }), idle).action).toBe('escape')
  })

  test('リピートでない最初の押下は通常どおり', () => {
    expect(resolveShortcut(key('e'), idle).action).toBe('archive')
  })
})

describe('DOM 判定', () => {
  test('入力要素', () => {
    const input = document.createElement('input')
    const area = document.createElement('textarea')
    const editor = document.createElement('div')
    editor.className = 'ProseMirror'
    const p = document.createElement('p')
    editor.append(p)
    const plain = document.createElement('div')
    for (const el of [input, area, editor, p]) expect(isEditableElement(el)).toBe(true)
    expect(isEditableElement(plain)).toBe(false)
    expect(isEditableElement(null)).toBe(false)
  })

  test('インタラクティブ要素', () => {
    const btn = document.createElement('button')
    const item = document.createElement('div')
    item.setAttribute('role', 'menuitem')
    const list = document.createElement('div')
    list.setAttribute('role', 'listbox')
    expect(isInteractiveElement(btn)).toBe(true)
    expect(isInteractiveElement(item)).toBe(true)
    expect(isInteractiveElement(list)).toBe(false)
  })

  test('開いているダイアログの検出', () => {
    expect(hasOpenDialog(document)).toBe(false)
    const d = document.createElement('div')
    d.setAttribute('role', 'dialog')
    document.body.append(d)
    expect(hasOpenDialog(document)).toBe(true)
    d.remove()
  })

  test('開いているポップアップの検出', () => {
    expect(hasOpenPopup(document)).toBe(false)
    const w = document.createElement('div')
    w.setAttribute('data-reka-popper-content-wrapper', '')
    document.body.append(w)
    expect(hasOpenPopup(document)).toBe(true)
    w.remove()
  })
})
