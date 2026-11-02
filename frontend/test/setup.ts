import { GlobalRegistrator } from '@happy-dom/global-registrator'
import { compileScript, parse } from '@vue/compiler-sfc'
import { plugin } from 'bun'

GlobalRegistrator.register()

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
