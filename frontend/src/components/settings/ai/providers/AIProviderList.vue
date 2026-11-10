<script setup lang="ts">
import { Check } from '@iconoir/vue'
import { computed, ref } from 'vue'
import { settings as msg } from '../../../../lib/messages'
import { useSettingsStore } from '../../../../stores/settings'
import { AI_PROVIDERS, type AIProviderId, hasProviderCredential } from './aiProviders'
import ProviderIcon from './ProviderIcon.vue'

// AI の設定の最初の画面（#315）。プロバイダを、アイコン・サービス名・補足のカードのグリッドにする。
// 押す（Android はタップ）とそのプロバイダの設定ページへ進む。選択中（チェック）は、保存済みの既定のプロバイダ
// （未保存の間は印なし）。カードは radiogroup の radio で、矢印キーでフォーカスを動かし（端で回り込む）、Enter・Space で開く。
// 矢印は選択（既定のプロバイダ）を変えない。既定は、プロバイダのページで保存したときにだけ変わる。
// 列数は CSS（repeat(auto-fill, minmax(min(160px, calc(50% - var(--s-3) / 2)), 1fr))）で、この一覧の幅から決まる。
const emit = defineEmits<{ open: [id: AIProviderId] }>()
const store = useSettingsStore()
const t = msg.ai.providerList

const buttons = ref<HTMLButtonElement[]>([])
const ids = AI_PROVIDERS.map((p) => p.id)
const selected = computed(() => ids.find((id) => store.aiConfig?.provider === id))
// Tab で入ったときにフォーカスされる 1 枚（roving tabindex）。選択中があればそれ、無ければ先頭。
const focusId = ref<AIProviderId | undefined>(undefined)
const tabStop = computed(() => focusId.value ?? selected.value ?? ids[0])

function note(id: AIProviderId): string {
  if (hasProviderCredential(id, store.aiConfig)) return t.noteConfigured
  return id === 'chatgpt' ? t.noteLogin : t.noteApiKey
}

function move(from: number, step: number): void {
  const next = (from + step + ids.length) % ids.length
  focusId.value = ids[next]
  buttons.value[next]?.focus()
}

function onKeydown(e: KeyboardEvent, i: number): void {
  const step = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[e.key]
  if (step !== undefined) {
    e.preventDefault()
    move(i, step)
  } else if (e.key === 'Home' || e.key === 'End') {
    e.preventDefault()
    move(e.key === 'Home' ? -1 : ids.length, e.key === 'Home' ? 1 : -1)
  }
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
    <div class="grid" role="radiogroup" :aria-label="t.listLabel" data-testid="ai-provider-grid">
      <!-- biome-ignore lint/a11y/useSemanticElements: カード全体（アイコン・名前・補足）を押せる radio にする仕様。input[type=radio] は中身を持てない -->
      <button
        v-for="(p, i) in AI_PROVIDERS"
        :key="p.id"
        :ref="(el) => { if (el) buttons[i] = el as HTMLButtonElement }"
        type="button"
        role="radio"
        class="card"
        :aria-checked="selected === p.id"
        :tabindex="tabStop === p.id ? 0 : -1"
        :data-testid="`ai-provider-${p.id}`"
        :aria-label="t.open(t.names[p.id] ?? p.id)"
        @click="emit('open', p.id)"
        @focus="focusId = p.id"
        @keydown="onKeydown($event, i)"
      >
        <Check v-if="selected === p.id" class="icon icon-sm check" aria-hidden="true" data-testid="ai-provider-default" />
        <ProviderIcon :id="p.id" />
        <span class="text">
          <span class="name">{{ t.names[p.id] }}</span>
          <span class="note" :data-testid="`ai-provider-status-${p.id}`">{{ note(p.id) }}</span>
        </span>
      </button>
    </div>
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
  /* 最小幅は 160px だが、狭いときは 2 列が収まるよう「幅の半分 − gap の半分」まで縮める（Android 390px の一覧 324px も 2 列）。 */
  grid-template-columns: repeat(auto-fill, minmax(min(160px, calc(50% - var(--s-3) / 2)), 1fr));
  gap: var(--s-3);
}

.card {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--s-3);
  width: 100%;
  min-height: 44px;
  padding: var(--s-4);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  color: var(--ink);
  text-align: left;
  cursor: pointer;
  /* #316 が足す --mute-on-soft が無いブランチでも動くよう、Light #5c5c5e／Dark #9c9c9f を既定にする（#316 のマージ後は不要）。 */
  --note-on-soft: var(--mute-on-soft, #5c5c5e);
  transition:
    border-color var(--motion-fast),
    background-color var(--motion-fast);
}

@media (prefers-color-scheme: dark) {
  .card {
    --note-on-soft: var(--mute-on-soft, #9c9c9f);
  }
}

.card:hover {
  border-color: var(--mute);
}

/* 選択中（既定のプロバイダ）: 地 --accent-soft・線 --ink 1.5px。線が太くなる分だけ padding を減らして、中身が動かないようにする。 */
.card[aria-checked='true'] {
  padding: calc(var(--s-4) - 0.5px);
  border-width: 1.5px;
  border-color: var(--ink);
  background: var(--accent-soft);
}

.card:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.check {
  position: absolute;
  top: var(--s-3);
  right: var(--s-3);
}

/* 名前と補足の間は --s-1。アイコンとの間（カードの gap）は --s-3 のまま、下に余白は足さない。 */
.text {
  display: flex;
  flex-direction: column;
  gap: var(--s-1);
  max-width: 100%;
}

.name {
  max-width: 100%;
  overflow: hidden;
  font-weight: 600;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.note {
  color: var(--mute);
  font-size: var(--font-size-xs);
  line-height: var(--line-height-xs);
}

/* 選択中のカードの頭文字タイルは、カードの地（--accent-soft）と同じ色で見えなくならないよう、地を --surface にする。 */
.card[aria-checked='true'] .provider-icon {
  background: var(--surface);
}

.card[aria-checked='true'] .note {
  color: var(--note-on-soft);
}
</style>
