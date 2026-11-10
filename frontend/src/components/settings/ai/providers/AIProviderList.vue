<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { settings as msg } from '../../../../lib/messages'
import { useSettingsStore } from '../../../../stores/settings'
import {
  AI_PROVIDERS,
  type AIProviderId,
  hasProviderCredential,
  providerGridColumns,
} from './aiProviders'
import ProviderIcon from './ProviderIcon.vue'

// AI の設定の最初の画面（#315）。プロバイダを、サービス名とアイコンのカードのグリッドにする。押す（Android はタップ）と
// そのプロバイダの設定ページへ進む。既定のプロバイダ（保存済みの設定。未保存は ChatGPT）には「既定」を付け、資格情報の状況をカードに出す。
// 列数はこの一覧の幅で決める（providerGridColumns。広いと 4 列、Android の幅は 2 列、さらに狭いと 1 列）。
const emit = defineEmits<{ open: [id: AIProviderId] }>()
const store = useSettingsStore()
const t = msg.ai.providerList

const grid = ref<HTMLElement>()
const columns = ref(providerGridColumns(0))
let observer: ResizeObserver | undefined

onMounted(() => {
  const el = grid.value
  if (!el) return
  // 最初の描画の前に測って、列数が後から変わってガタつかないようにする。
  columns.value = providerGridColumns(el.clientWidth)
  if (typeof ResizeObserver !== 'function') return
  observer = new ResizeObserver((entries) => {
    const w = entries[entries.length - 1]?.contentRect.width ?? el.clientWidth
    columns.value = providerGridColumns(w)
  })
  observer.observe(el)
})

onUnmounted(() => observer?.disconnect())

function status(id: AIProviderId): string {
  const has = hasProviderCredential(id, store.aiConfig)
  if (id === 'chatgpt') return has ? t.statusSignedIn : t.statusSignedOut
  return has ? t.statusKeySet : t.statusKeyUnset
}
</script>

<template>
  <section class="providers" :aria-label="msg.ai.heading">
    <div class="group">
      <h3 class="heading">{{ msg.ai.heading }}</h3>
      <p class="intro">{{ msg.ai.intro }}</p>
    </div>
    <div class="group">
      <h4 class="sub">{{ t.heading }}</h4>
      <p class="hint">{{ t.intro }}</p>
    </div>
    <ul
      ref="grid"
      class="grid"
      :style="{ '--cols': columns }"
      :data-columns="columns"
      :aria-label="t.listLabel"
      data-testid="ai-provider-grid"
    >
      <li v-for="p in AI_PROVIDERS" :key="p.id">
        <button
          type="button"
          class="card"
          :class="{ current: store.aiConfig?.provider === p.id }"
          :data-testid="`ai-provider-${p.id}`"
          :aria-label="t.open(t.names[p.id] ?? p.id)"
          @click="emit('open', p.id)"
        >
          <ProviderIcon :id="p.id" />
          <span class="name">{{ t.names[p.id] }}</span>
          <span
            v-if="store.aiConfig?.provider === p.id"
            class="badge"
            data-testid="ai-provider-default"
          >{{ t.defaultBadge }}</span>
          <span class="status" :data-testid="`ai-provider-status-${p.id}`">{{ status(p.id) }}</span>
        </button>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.providers {
  display: flex;
  flex-direction: column;
  gap: var(--gap-field);
}

.group {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

.heading {
  margin: 0;
  font-weight: 600;
  font-size: var(--font-size-base);
}

.sub {
  margin: 0;
  font-weight: 600;
  font-size: var(--font-size-sm);
}

.intro {
  margin: 0;
  color: var(--ink);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.hint {
  margin: 0;
  color: var(--mute);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.grid {
  display: grid;
  grid-template-columns: repeat(var(--cols, 1), minmax(0, 1fr));
  gap: var(--s-3);
  margin: 0;
  padding: 0;
  list-style: none;
}

.card {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--s-2);
  width: 100%;
  height: 100%;
  min-height: 44px;
  padding: var(--pad-card-sm);
  border: 1px solid var(--border);
  border-radius: var(--r-lg);
  background: var(--surface);
  color: var(--ink);
  text-align: left;
  cursor: pointer;
}

.card:hover {
  background: var(--accent-soft);
}

/* 既定のプロバイダ: 「既定」の札に加えて、枠も --accent にする（色だけで伝えない）。 */
.card.current {
  border-color: var(--accent);
}

.name {
  font-weight: 600;
  font-size: var(--font-size-base);
  overflow-wrap: anywhere;
}

.badge {
  padding: 0 var(--s-2);
  border: 1px solid var(--border);
  border-radius: var(--r-full);
  font-size: var(--font-size-xs);
}

.status {
  margin-top: auto;
  /* hover の --accent-soft 地の上でも AA を保つため、--mute ではなく --ink。 */
  color: var(--ink);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  overflow-wrap: anywhere;
}
</style>
