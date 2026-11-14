<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { parseBindingError } from '../../../../lib/api'
import { settings as msg } from '../../../../lib/messages'
import { useSettingsStore } from '../../../../stores/settings'
import { useUiStore } from '../../../../stores/ui'
import BaseButton from '../../../ui/BaseButton.vue'
import AIStatusChip from '../providers/AIStatusChip.vue'
import { AI_PROVIDERS, isConnected } from '../providers/aiProviders'
import AIFeatureRow from './AIFeatureRow.vue'
import { modelsKeyFor } from './aiFeatures'
import InboxZeroReasons from './InboxZeroReasons.vue'

// 「機能ごとに使う AI」タブ（#376）。機能ごとに 1 行（AI で下書き・Inbox Zero・エージェント＝準備中）。
// 何もつないでいない（選んでいるものも無い）ときは、一覧の代わりにひとつだけ案内を出して「つなぎ先」へ誘導する。
// 一覧の下に、Inbox Zero で不要とみなす理由のオン・オフ（#377）。AI を使わない理由（返信済み・カレンダー）だけなら、AI をつながなくても使える。
const store = useSettingsStore()
const ui = useUiStore()
const t = msg.ai.use

const cfg = computed(() => store.aiConfig)
const connectedModelProviders = computed(() =>
  AI_PROVIDERS.map((p) => p.id).filter((id) => id !== 'typesafe' && isConnected(id, cfg.value)),
)
const anySaved = computed(() => {
  const f = cfg.value?.features
  return f !== undefined && (f.draft.provider !== '' || f.inboxZero.provider !== '')
})
const anyConnected = computed(() => AI_PROVIDERS.some((p) => isConnected(p.id, cfg.value)))
const showEmpty = computed(() => cfg.value !== undefined && !anyConnected.value && !anySaved.value)

/** つないであるプロバイダのモデル一覧を取得する。取得できないときは、そのプロバイダは手入力にする（理由つきの空の一覧を入れる）。 */
async function loadModels(): Promise<void> {
  const c = cfg.value
  if (c === undefined) return
  await Promise.all(
    connectedModelProviders.value.map(async (p) => {
      try {
        await store.loadAIModels(p, p === 'openai' ? (c.baseUrl ?? '') : '')
      } catch (e) {
        store.aiModels = {
          ...store.aiModels,
          [modelsKeyFor(p, c)]: { provider: p, models: [], reason: 'failed', message: parseBindingError(e).message },
        }
      }
    }),
  )
}
// つないであるプロバイダ（と OpenAI 互換の Base URL）が変わったら取り直す。
watch(() => [connectedModelProviders.value.join(','), cfg.value?.baseUrl], () => void loadModels(), { immediate: true })
onMounted(() => {
  if (cfg.value === undefined) void store.load()
})

function goConnect(): void {
  ui.aiTab = 'connect'
}
function fix(provider: string): void {
  ui.openAiProvider(provider, true)
}
</script>

<template>
  <section class="use" :aria-label="t.heading" data-testid="ai-use-tab">
    <div class="group">
      <h4 class="sh">{{ t.heading }}</h4>
      <p v-if="!showEmpty" class="hint">{{ t.hint }}</p>
    </div>
    <div v-if="showEmpty" class="empty" data-testid="ai-use-empty">
      <p class="empty-text">{{ t.empty }}</p>
      <BaseButton variant="secondary" size="sm" data-testid="ai-use-connect" @click="goConnect">{{ t.connect }}</BaseButton>
    </div>
    <ul v-else class="box" :aria-label="t.listLabel" data-testid="ai-feature-list">
      <AIFeatureRow
        feature="draft"
        :label="t.features.draft"
        @connect="(p) => (p ? fix(p) : goConnect())"
      />
      <AIFeatureRow
        feature="inboxZero"
        :label="t.features.inboxZero"
        @connect="(p) => (p ? fix(p) : goConnect())"
      />
      <li class="agent" aria-disabled="true" data-testid="ai-feature-agent">
        <div class="alab">
          {{ t.features.agent }}
          <AIStatusChip kind="wait">{{ t.soon }}</AIStatusChip>
        </div>
      </li>
    </ul>
    <InboxZeroReasons />
  </section>
</template>

<style scoped>
.use {
  display: flex;
  flex-direction: column;
  gap: var(--s-3);
}

.group {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

.sh {
  margin: 0;
  font-weight: 600;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.hint {
  margin: 0;
  color: var(--mute);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.box {
  margin: 0;
  padding: 0;
  list-style: none;
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
}

.box > :deep(* + *) {
  border-top: 1px solid var(--border);
}

.empty {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--s-3);
  padding: var(--s-6) var(--s-4);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  line-break: strict;
  word-break: keep-all;
}

.empty-text {
  margin: 0;
  color: var(--mute);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

/* エージェント: 準備中。選べない（保存もしない）ので、行ごと薄くして押せなくする。 */
.agent {
  display: flex;
  align-items: center;
  min-height: 48px;
  padding: var(--s-2) var(--s-4);
  opacity: 0.4;
}

.alab {
  display: flex;
  align-items: center;
  gap: var(--s-2);
  font-weight: 500;
  font-size: var(--font-size-sm);
}

/* 390px 幅: 行はそれぞれ枠のあるカードにし、一覧の外枠は外す。 */
@media (max-width: 640px) {
  .box {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    border: none;
    background: transparent;
  }

  .box > :deep(*) {
    border: 1px solid var(--border);
    border-radius: var(--r-md);
    background: var(--surface);
  }

  .box > :deep(* + *) {
    border-top: 1px solid var(--border);
  }

  .agent {
    padding: var(--s-3);
  }
}
</style>
