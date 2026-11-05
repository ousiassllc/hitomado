import { expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { Glob } from 'bun'

// 回帰: XML のコメントに `--`（CSS のトークン名 `--accent` など）を書くと XML 仕様違反で、
// Android のリソースマージ（Resource and asset merger）が「The string "--" is not permitted within comments」で落ちる。
// フロントのテストだけでは気づけず CI の android ジョブまで届いていた。
const ANDROID_SRC = join(import.meta.dir, '../../../build/android/app/src')

test('Android の XML のコメントに `--` を含まない（コメントの末尾が `-` でもない）', () => {
  const offenders: string[] = []
  const files = [...new Glob('**/*.xml').scanSync({ cwd: ANDROID_SRC })]
  expect(files.length).toBeGreaterThan(0)
  for (const rel of files) {
    const xml = readFileSync(join(ANDROID_SRC, rel), 'utf8')
    for (const m of xml.matchAll(/<!--([\s\S]*?)-->/g)) {
      const body = m[1] ?? ''
      if (body.includes('--') || body.endsWith('-')) {
        offenders.push(`${rel}:${xml.slice(0, m.index).split('\n').length}`)
      }
    }
  }
  expect(offenders).toEqual([])
})
