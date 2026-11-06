<script setup lang="ts">
import { Search as SearchIcon, Xmark } from '@iconoir/vue'
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
      <div class="field">
        <SearchIcon class="icon field-icon" aria-hidden="true" />
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
      </div>
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

.field {
  position: relative;
  flex: 1;
  min-width: 0;
}

.field-icon {
  position: absolute;
  top: 50%;
  left: var(--s-3);
  color: var(--mute);
  pointer-events: none;
  transform: translateY(-50%);
}

.input {
  width: 100%;
  min-width: 0;
  height: var(--control-h);
  padding: 0 var(--s-3);
  /* 左の検索アイコン（--icon-md）の分だけ空ける */
  padding-left: calc(var(--s-3) + var(--icon-md) + var(--s-2));
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  color: var(--ink);
  font-size: var(--font-size-sm);
}

/* ブラウザ標準の✕は出さない（O8）。クリアは右の自前のボタンに一本化する。 */
.input::-webkit-search-cancel-button {
  appearance: none;
}

.input:focus {
  border-color: var(--focus);
  box-shadow: 0 0 0 2px var(--ring);
  outline: none;
}
</style>
