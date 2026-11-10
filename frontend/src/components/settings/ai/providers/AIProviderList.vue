<script setup lang="ts">
import { NavArrowRight } from '@iconoir/vue'
import { settings as msg } from '../../../../lib/messages'
import { useSettingsStore } from '../../../../stores/settings'
import { AI_PROVIDERS, type AIProviderId, hasProviderCredential } from './aiProviders'

// AI の設定の最初の画面（#315）。プロバイダを一覧にし、押す（Android はタップ）とそのプロバイダの設定ページへ進む。
// 既定のプロバイダ（保存済みの設定。未保存は ChatGPT）には「既定」を付け、資格情報の状況を各行に出す。
const emit = defineEmits<{ open: [id: AIProviderId] }>()
const store = useSettingsStore()
const t = msg.ai.providerList

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
    <ul class="list" :aria-label="t.listLabel">
      <li v-for="p in AI_PROVIDERS" :key="p.id">
        <button
          type="button"
          class="card"
          :data-testid="`ai-provider-${p.id}`"
          :aria-label="t.open(t.names[p.id] ?? p.id)"
          @click="emit('open', p.id)"
        >
          <span class="body">
            <span class="name-row">
              <span class="name">{{ t.names[p.id] }}</span>
              <span
                v-if="store.aiConfig?.provider === p.id"
                class="badge"
                data-testid="ai-provider-default"
              >{{ t.defaultBadge }}</span>
            </span>
            <span class="summary">{{ t.summaries[p.id] }}</span>
            <span class="status" :data-testid="`ai-provider-status-${p.id}`">{{ status(p.id) }}</span>
          </span>
          <NavArrowRight class="icon chevron" aria-hidden="true" />
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

.list {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--s-3);
  width: 100%;
  min-height: 44px;
  padding: var(--s-3);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  color: var(--ink);
  text-align: left;
  cursor: pointer;
}

.card:hover {
  background: var(--accent-soft);
}

.body {
  display: flex;
  flex-direction: column;
  gap: var(--s-1);
  min-width: 0;
}

.name-row {
  display: flex;
  align-items: center;
  gap: var(--s-2);
}

.name {
  font-weight: 600;
  font-size: var(--font-size-base);
}

.badge {
  padding: 0 var(--s-2);
  border: 1px solid var(--border);
  border-radius: var(--r-full);
  font-size: var(--font-size-xs);
}

.summary,
.status {
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  overflow-wrap: anywhere;
}

.status {
  color: var(--ink);
}

.chevron {
  flex: none;
}
</style>
