<script setup lang="ts">
import { agent } from '../../lib/messages'
import BaseDisclosure from '../ui/BaseDisclosure.vue'

// エージェントのパネルの区分（質問・確認待ち・あとで・返信不要・失敗）。件数つきの見出しと、行（スロット）の並び。
// collapsible は折りたたみ（既定で閉じる。あとで・返信不要のように、急がないもの）。0 件なら出さない。
defineProps<{ title: string; count: number; collapsible?: boolean }>()
</script>

<template>
  <section v-if="count > 0" class="section" :aria-label="agent.panel.count(title, count)">
    <BaseDisclosure v-if="collapsible" :summary="agent.panel.count(title, count)">
      <ul class="list"><slot /></ul>
    </BaseDisclosure>
    <template v-else>
      <h3 class="heading">{{ agent.panel.count(title, count) }}</h3>
      <ul class="list"><slot /></ul>
    </template>
  </section>
</template>

<style scoped>
.section {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

.heading {
  margin: 0;
  color: var(--mute);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  font-weight: 600;
}

.list {
  display: flex;
  flex-direction: column;
  gap: var(--s-3);
  margin: 0;
  padding: 0;
}
</style>
