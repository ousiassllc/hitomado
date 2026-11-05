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
