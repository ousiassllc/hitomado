import { describe, expect, test } from 'bun:test'
import { droppedPaths, FILES_DROPPED_EVENT } from '../../src/lib/dropFiles'

describe('droppedPaths', () => {
  test('イベント名は Go 側（appevents.FilesDropped）と同じ files-dropped', () => {
    expect(FILES_DROPPED_EVENT).toBe('files-dropped')
  })

  test('ペイロードの形ごとの取り出し', () => {
    const table: [string, unknown, string[]][] = [
      ['単体の {files}', { files: ['/a.txt', '/b.txt'] }, ['/a.txt', '/b.txt']],
      ['配列で包んだ {files}（Wails のイベントの形）', [{ files: ['/a.txt'] }], ['/a.txt']],
      ['files が空', { files: [] }, []],
      ['包んだ files が空', [{ files: [] }], []],
      ['files が無い', {}, []],
      ['files が配列でない', { files: '/a.txt' }, []],
      ['空配列', [], []],
      ['null', null, []],
      ['undefined', undefined, []],
      ['文字列', '/a.txt', []],
      ['数値', 1, []],
      ['文字列だけの配列', ['/a.txt'], []],
      ['要素が 2 つ以上の配列', [{ files: ['/a.txt'] }, { files: ['/b.txt'] }], []],
      ['null を包んだ配列', [null], []],
      [
        '文字列以外の要素は捨てる',
        { files: ['/a.txt', 1, null, { x: 1 }, '/b.txt'] },
        ['/a.txt', '/b.txt'],
      ],
      ['空文字のパスは捨てる', { files: ['', '/a.txt'] }, ['/a.txt']],
    ]
    for (const [name, data, want] of table)
      expect({ name, got: droppedPaths(data) }).toEqual({ name, got: want })
  })
})
