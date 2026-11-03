// リポジトリ直下の NOTICE（同梱する第三者ソフトウェアのライセンス表記）を作る。
//
//   bun .github/scripts/gen-notice.ts          # NOTICE を書き換える
//   bun .github/scripts/gen-notice.ts --check  # NOTICE が最新でなければ失敗する（書き換えない）
//
// 対象:
//   - Go: windows/amd64 の exe に実際にリンクされるモジュール（`go list -deps`）と Go 標準ライブラリ
//   - frontend: frontend/package.json の dependencies から bun.lock をたどった本番用の依存（devDependencies は含めない）
// 前提: モジュールキャッシュ（go mod download 済み）と frontend/node_modules（bun install --frozen-lockfile 済み）。
// 出力は入力（go.mod / bun.lock と各パッケージの LICENSE）だけで決まる。ライセンスの種別が判定できない依存があれば失敗する
// （内容を確認して下の classify に規則を足す）。
import { createHash } from 'node:crypto'
import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'

const root = resolve(import.meta.dir, '../..')
const noticePath = join(root, 'NOTICE')
const REPO = 'https://github.com/ousiassllc/hitomado'

type Component = {
  kind: 'Go' | 'npm'
  name: string
  version: string
  license: string
  files: string[] // ライセンス全文のファイルの絶対パス
  source: string // 配布元（MPL の入手先に使う）
  tarball: string // その版のソース一式の入手先
}

const normalize = (s: string) => s.replace(/\r\n?/g, '\n').replace(/^\uFEFF/, '').trim()

function classify(text: string): string | null {
  text = text.replace(/\s+/g, ' ')
  if (/Mozilla Public License,? Version 2\.0/i.test(text)) return 'MPL-2.0'
  if (/Apache License\s+Version 2\.0/i.test(text)) return 'Apache-2.0'
  if (/This is free and unencumbered software/i.test(text)) return 'Unlicense'
  if (/SQLite Is Public Domain/i.test(text)) return 'Public-Domain'
  if (/Permission is hereby granted, free of charge/i.test(text)) return 'MIT'
  if (/Permission to use, copy, modify, and\/or distribute this software for any purpose/i.test(text))
    return 'ISC'
  if (/Redistribution and use in source and binary forms/i.test(text))
    return /Neither the name|endorse or promote/i.test(text) ? 'BSD-3-Clause' : 'BSD-2-Clause'
  return null
}

// LICENSE-LOGO は modernc.org のロゴ画像用で、exe には入らないので対象外。
const LICENSE_FILE = /^(licen[cs]e|copying|notice|patents)(?!-logo$)([-._][a-z0-9._-]*)?$/i
const licenseFilesIn = (dir: string) =>
  readdirSync(dir)
    .filter((f) => LICENSE_FILE.test(f) && !f.endsWith('.go') && statSync(join(dir, f)).isFile())
    .sort()
    .map((f) => join(dir, f))

// 全文ファイルから種別を決める（NOTICE・PATENTS は種別の判定に使わない）。
function licenseName(files: string[]): string {
  const names = new Set<string>()
  for (const f of files) {
    if (/^(notice|patents)|3rd-party/i.test(f.split('/').pop() ?? '')) continue
    const n = classify(readFileSync(f, 'utf8'))
    if (!n) throw new Error(`ライセンスの種別を判定できません: ${f}`)
    names.add(n)
  }
  return [...names].sort().join(' + ')
}

async function sh(cmd: string[], env: Record<string, string> = {}): Promise<string> {
  const p = Bun.spawn(cmd, { cwd: root, env: { ...process.env, ...env }, stdout: 'pipe', stderr: 'inherit' })
  const out = await new Response(p.stdout).text()
  if ((await p.exited) !== 0) throw new Error(`${cmd.join(' ')} が失敗しました`)
  return out
}

const goEscape = (p: string) => p.replace(/[A-Z]/g, (c) => `!${c.toLowerCase()}`)

async function goComponents(): Promise<Component[]> {
  const format = '{{if .Module}}{{if not .Module.Main}}{{.Module.Path}}\t{{.Module.Version}}\t{{.Module.Dir}}\t{{.Dir}}{{end}}{{end}}'
  // GOFLAGS を空にして、実行環境のビルドタグ等に左右されないようにする（リリースと同じ windows/amd64・production）。
  const out = await sh(['go', 'list', '-deps', '-tags', 'production', '-f', format, '.'], {
    GOOS: 'windows',
    GOARCH: 'amd64',
    CGO_ENABLED: '0',
    GOFLAGS: '',
  })
  const modules = new Map<string, { version: string; dir: string; pkgDirs: Set<string> }>()
  for (const line of out.split('\n')) {
    if (!line) continue
    const [path, version, dir, pkgDir] = line.split('\t')
    const m = modules.get(path) ?? { version, dir, pkgDirs: new Set() }
    m.pkgDirs.add(pkgDir)
    modules.set(path, m)
  }
  if (!modules.has('github.com/hashicorp/golang-lru/v2')) throw new Error('go list の結果に golang-lru が無い')

  const components: Component[] = []
  for (const [path, m] of modules) {
    // パッケージのディレクトリからモジュールの最上位までのライセンスファイルを集める。
    const dirs = new Set<string>()
    for (let d of m.pkgDirs) {
      for (; ; d = dirname(d)) {
        dirs.add(d)
        if (d === m.dir) break
      }
    }
    const files = [...dirs].sort().flatMap(licenseFilesIn)
    if (files.length === 0) throw new Error(`${path} にライセンスファイルがありません`)
    const repo = path.split('/').slice(0, 3).join('/')
    components.push({
      kind: 'Go',
      name: path,
      version: m.version,
      license: licenseName(files),
      files,
      source: path.startsWith('github.com/') ? `https://${repo}/tree/${m.version}` : `https://pkg.go.dev/${path}@${m.version}`,
      tarball: `https://proxy.golang.org/${goEscape(path)}/@v/${goEscape(m.version)}.zip`,
    })
  }
  const goroot = (await sh(['go', 'env', 'GOROOT'])).trim()
  components.push({
    kind: 'Go',
    name: 'Go standard library',
    version: '',
    license: 'BSD-3-Clause',
    files: [join(goroot, 'LICENSE')],
    source: 'https://go.dev/',
    tarball: 'https://go.dev/dl/',
  })
  return components
}

type LockEntry = [string, string, { dependencies?: Record<string, string>; optionalDependencies?: Record<string, string>; peerDependencies?: Record<string, string>; optionalPeers?: string[] }, string?]

// パッケージにライセンスファイルが無いとき、同じプロジェクトの Go モジュールの LICENSE を使う（宣言は package.json の license）。
const SHARED_LICENSE: Record<string, string> = { '@wailsio/runtime': 'github.com/wailsapp/wails/v3' }

function npmComponents(): Component[] {
  const front = join(root, 'frontend')
  const lock = Bun.JSONC.parse(readFileSync(join(front, 'bun.lock'), 'utf8')) as {
    workspaces: { '': { dependencies: Record<string, string> } }
    packages: Record<string, LockEntry>
  }
  // 親のキーの下に入れ子で入っている版を先に探し、無ければ上へたどる（node の解決と同じ）。
  const resolveKey = (parent: string, dep: string): string | null => {
    for (let p = parent; ; p = p.includes('/') ? p.slice(0, p.lastIndexOf('/')) : '') {
      const key = p ? `${p}/${dep}` : dep
      if (lock.packages[key]) return key
      if (!p) return null
    }
  }
  const dirOf = (key: string): string => {
    const name = lock.packages[key][0].replace(/@[^@]*$/, '')
    return key === name ? join(front, 'node_modules', name) : join(dirOf(key.slice(0, key.length - name.length - 1)), 'node_modules', name)
  }
  const seen = new Set<string>()
  const visit = (parent: string, dep: string, optional: boolean) => {
    const key = resolveKey(parent, dep)
    if (!key) {
      if (optional) return
      throw new Error(`bun.lock に ${dep} がありません`)
    }
    if (seen.has(key)) return
    seen.add(key)
    const meta = lock.packages[key][2]
    for (const d of Object.keys(meta.dependencies ?? {})) visit(key, d, false)
    for (const d of Object.keys(meta.peerDependencies ?? {})) {
      if (!meta.optionalPeers?.includes(d)) visit(key, d, true)
    }
    for (const d of Object.keys(meta.optionalDependencies ?? {})) visit(key, d, true)
  }
  for (const d of Object.keys(lock.workspaces[''].dependencies)) visit('', d, false)

  const components: Component[] = []
  for (const key of seen) {
    const dir = dirOf(key)
    if (!existsSync(join(dir, 'package.json'))) throw new Error(`${dir} がありません（bun install --frozen-lockfile を実行する）`)
    const pkg = JSON.parse(readFileSync(join(dir, 'package.json'), 'utf8'))
    const files = licenseFilesIn(dir)
    const declared = typeof pkg.license === 'string' ? pkg.license : pkg.license?.type
    if (files.length === 0 && !SHARED_LICENSE[pkg.name]) throw new Error(`${pkg.name} にライセンスファイルがありません`)
    const repo = String(typeof pkg.repository === 'string' ? pkg.repository : (pkg.repository?.url ?? ''))
      .replace(/^git\+/, '').replace(/\.git$/, '').replace(/^git:\/\//, 'https://').replace(/^github:/, 'https://github.com/')
    components.push({
      kind: 'npm',
      name: pkg.name,
      version: pkg.version,
      license: (declared ?? licenseName(files)).replace(/^\((.*)\)$/, '$1'),
      files,
      source: repo || `https://www.npmjs.com/package/${pkg.name}/v/${pkg.version}`,
      tarball: `https://registry.npmjs.org/${pkg.name}/-/${pkg.name.split('/').pop()}-${pkg.version}.tgz`,
    })
  }
  return components
}

function render(components: Component[]): string {
  const byName = (a: Component, b: Component) => (a.kind + a.name < b.kind + b.name ? -1 : a.kind + a.name > b.kind + b.name ? 1 : 0)
  components.sort(byName)
  const label = (c: Component) => `${c.name}${c.version ? ` ${c.version}` : ''}`
  const mpl = components.filter((c) => /MPL-2\.0/.test(c.license))

  const texts = new Map<string, { text: string; users: Set<string> }>()
  for (const c of components) {
    for (const f of c.files) {
      const text = normalize(readFileSync(f, 'utf8'))
      const id = createHash('sha256').update(text).digest('hex')
      const t = texts.get(id) ?? { text, users: new Set() }
      t.users.add(`${c.kind}: ${label(c)}`)
      texts.set(id, t)
    }
  }

  const rule = '='.repeat(78)
  const out: string[] = []
  out.push(
    'Hitomado - Third-Party Notices',
    '',
    'Hitomado is free software licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).',
    `The license text is in the LICENSE file of the source repository: ${REPO}/blob/main/LICENSE`,
    `The source code is available at: ${REPO}`,
    '',
    'Hitomado includes the third-party software listed below. Each component is distributed under its own',
    'license; the license texts are reproduced in the last section. This file is generated by',
    '.github/scripts/gen-notice.ts - do not edit it by hand.',
    '',
    rule,
    'Source code availability for MPL-2.0 components',
    rule,
    '',
  )
  if (mpl.length === 0) out.push('(none)', '')
  else {
    out.push(
      'The components below are licensed under the Mozilla Public License 2.0 (MPL-2.0), alone or as one',
      'of the licenses to choose from. Hitomado does not modify their source code. The source code of the',
      'exact version that is included is available from the URLs below. The MPL-2.0 text is at',
      'https://www.mozilla.org/MPL/2.0/ and in the last section.',
      '',
    )
    for (const c of mpl) out.push(`- ${label(c)} (${c.license})`, `    repository: ${c.source}`, `    source archive: ${c.tarball}`)
    out.push('')
  }
  out.push(rule, 'Components', rule, '')
  for (const c of components) out.push(`- [${c.kind}] ${label(c)} - ${c.license}`)
  out.push('', rule, 'License texts', rule)
  for (const { text, users } of texts.values()) {
    out.push('', '-'.repeat(78), 'Used by:', ...[...users].sort().map((u) => `  ${u}`), '-'.repeat(78), '', text)
  }
  return `${out.join('\n')}\n`
}

const goList = await goComponents()
const npmList = npmComponents()
for (const c of npmList) {
  const shared = SHARED_LICENSE[c.name]
  if (c.files.length === 0) c.files = goList.find((g) => g.name === shared)?.files.filter((f) => f.endsWith('/LICENSE')).slice(0, 1) ?? []
  if (c.files.length === 0) throw new Error(`${c.name} のライセンス全文が見つかりません`)
}
const notice = render([...goList, ...npmList])
if (process.argv.includes('--check')) {
  if (!existsSync(noticePath) || readFileSync(noticePath, 'utf8') !== notice) {
    console.error('NOTICE is out of date. Run: bun .github/scripts/gen-notice.ts')
    process.exit(1)
  }
} else {
  await Bun.write(noticePath, notice)
}
