<script setup lang="ts">
import { Xmark } from '@iconoir/vue'
import { ref, watch } from 'vue'
import { common } from '../../lib/messages'
import BaseIconButton from '../ui/BaseIconButton.vue'

const props = defineProps<{ active: boolean; query: string }>()
const emit = defineEmits<{ search: [q: string]; clear: [] }>()

const text = ref(props.query)
const input = ref<HTMLInputElement | null>(null)

// 検索クリア（Esc／g→i／受信箱に戻る）に入力欄を追従させる
watch(
  () => props.query,
  (q) => {
    text.value = q
  },
)

function onEnter(): void {
  const q = text.value.trim()
  if (q) emit('search', q)
  else emit('clear')
}

function onEscape(): void {
  // 入力欄の Esc: 入力があれば消去、検索中ならクリア、いずれも無ければフォーカスを外す
  if (text.value !== '' || props.active) {
    text.value = ''
    if (props.active) emit('clear')
  } else {
    input.value?.blur()
  }
}
</script>

<template>
  <search class="search">
    <form class="form" @submit.prevent="onEnter">
    <input
      ref="input"
      v-model="text"
      class="input"
      type="search"
      :placeholder="common.search.placeholder"
      :aria-label="common.search.label"
      data-search-input
      autocomplete="off"
      spellcheck="false"
      @keydown.esc.stop.prevent="onEscape"
    />
    <BaseIconButton v-if="active" :label="common.search.clear" @click="emit('clear')"><Xmark class="icon" aria-hidden="true" /></BaseIconButton>
    </form>
  </search>
</template>

<style scoped>
.search {
  display: block;
  border-bottom: 1px solid var(--border);
}

.form {
  display: flex;
  align-items: center;
  gap: var(--s-1);
  padding: var(--s-2) var(--s-3);
}

.input {
  flex: 1;
  min-width: 0;
  height: var(--control-h);
  padding: 0 var(--s-3);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  color: var(--ink);
  font-size: var(--font-size-sm);
}
</style>
