<script setup lang="ts">
import { ChatBubble, Cpu, Mail, Sparks } from '@iconoir/vue'
import type { Component } from 'vue'
import type { AIProviderId } from './aiProviders'

// プロバイダのアイコン枠（#315）。公式ロゴを使うか汎用アイコンにするかは未定（ブランドの決まりの確認待ち）なので、
// 一覧のカードはこの 1 か所だけを通してアイコンを出す。今は汎用の Iconoir アイコンを枠に入れた仮のもの。
// 差し替えるときは、ここの ICONS（または下の <component :is>）を、公式ロゴの <img>・SVG に替えるだけでよい（一覧側は変えない）。
// 装飾なので読み上げない（カードにはサービス名の文字がある）。
defineProps<{ id: AIProviderId }>()

const ICONS: Record<AIProviderId, Component> = {
  chatgpt: ChatBubble,
  anthropic: Sparks,
  openai: Cpu,
  typesafe: Mail,
}
</script>

<template>
  <span class="provider-icon" :data-provider-icon="id" aria-hidden="true">
    <component :is="ICONS[id]" class="icon icon-lg" />
  </span>
</template>

<style scoped>
.provider-icon {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: var(--s-12);
  height: var(--s-12);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--bone);
  color: var(--ink);
}
</style>
