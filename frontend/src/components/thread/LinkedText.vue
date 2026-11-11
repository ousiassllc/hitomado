<script setup lang="ts">
import { computed } from 'vue'
import { splitLinks } from '../../lib/autolink'

// テキストメールの本文（親の `<pre>` の中身）。素の URL・メールアドレスを `<a>` にする（#312）。href は http／https／mailto だけ。
// クリックは常にナビゲーションを止め、親が既定のブラウザで開く（iframe 内のリンクと同じ扱い）。
const props = defineProps<{ text: string }>()
const emit = defineEmits<{ link: [url: string] }>()

const segments = computed(() => splitLinks(props.text))
</script>

<template>
  <template v-for="(seg, i) in segments" :key="i"><a v-if="seg.href" class="body-link" :href="seg.href" rel="noopener noreferrer" @click.prevent="emit('link', seg.href)">{{ seg.text }}</a><template v-else>{{ seg.text }}</template></template>
</template>

<style scoped>
.body-link {
  color: var(--ink);
  text-decoration: underline;
  text-decoration-color: var(--mute);
  text-underline-offset: 2px;
  cursor: pointer;
}
</style>
