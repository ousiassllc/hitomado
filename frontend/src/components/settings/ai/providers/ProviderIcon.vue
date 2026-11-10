<script setup lang="ts">
import { Server } from '@iconoir/vue'
import { computed } from 'vue'
import type { AIProviderId } from './aiProviders'

// プロバイダのアイコン枠（#315）。一覧のカードは、アイコンを必ずこのコンポーネント 1 つを通して出す。
// 今は公式ロゴを使わず、頭文字のタイル（C・A・T。色分けなし）と、頭文字の無い OpenAI 互換だけ Iconoir の server。
// 公式ロゴに差し替えるときは、ここの TILES（と下の <template> の中身）だけを替える。一覧側は変えない。
// 装飾なので読み上げない（カードにはサービス名の文字がある）。
const props = defineProps<{ id: AIProviderId }>()

const TILES: Record<AIProviderId, string | undefined> = {
  chatgpt: 'C',
  anthropic: 'A',
  typesafe: 'T',
  openai: undefined,
}
const initial = computed(() => TILES[props.id])
</script>

<template>
  <span class="provider-icon" :data-provider-icon="id" aria-hidden="true">
    <template v-if="initial">{{ initial }}</template>
    <Server v-else class="icon" />
  </span>
</template>

<style scoped>
.provider-icon {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 6px;
  background: var(--accent-soft);
  color: var(--ink);
  font-family: var(--font-sans);
  font-weight: 600;
  font-size: var(--font-size-sm);
  line-height: 1;
}
</style>
