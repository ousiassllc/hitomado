import { GlobalRegistrator } from '@happy-dom/global-registrator'
import { compileScript, parse } from '@vue/compiler-sfc'
import { plugin } from 'bun'

GlobalRegistrator.register()

// アイコン（Iconoir）は App.vue の IconoirProvider から既定値を受ける。単体でマウントするテストでは無い（なくても描ける）ので、その警告だけ出さない。
// Vue（runtime-dom）は読み込み時に document を見るので、DOM を登録したあとで読み込む。
const { config } = await import('@vue/test-utils')
config.global.config.warnHandler = (msg, _instance, trace) => {
  if (msg.includes('IconoirProvider')) return
  console.warn(`[Vue warn]: ${msg}${trace}`)
}

// DOMPurify は最初に使われたときの happy-dom の状態で作られて使い回される（src/lib/sanitize）。補正を各テストの
// import に任せると、読み込まないテストが先に本文を描いたときサニタイズが効かないまま後続が走る（ファイルの
// 実行順は環境で異なる）。補正は読み込み時に document を見るので、DOM の登録後に動的に読み込む（静的 import は巻き上げられる）。
await import('./domPurifyShim')

// bun test が .vue（<script setup lang="ts"> ＋ template）を import できるようにする SFC ローダー。
const transpiler = new Bun.Transpiler({ loader: 'ts' })

plugin({
  name: 'vue-sfc',
  setup(build) {
    build.onLoad({ filter: /\.vue$/ }, async ({ path }) => {
      const source = await Bun.file(path).text()
      const { descriptor, errors } = parse(source, { filename: path })
      if (errors.length > 0) throw errors[0]
      const id = Bun.hash(path).toString(16)
      const script = compileScript(descriptor, { id, inlineTemplate: true })
      return { contents: transpiler.transformSync(script.content), loader: 'js' }
    })
  },
})
