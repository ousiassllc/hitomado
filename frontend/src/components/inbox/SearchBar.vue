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

function onClear(): void {
  // 入力欄の右端のクリア: 入力を空にし、検索中なら検索も解除する。続けて打てるようフォーカスは入力欄へ戻す
  text.value = ''
  if (props.active) emit('clear')
  input.value?.focus()
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
          :class="{ 'has-clear': text !== '' }"
          type="search"
          :placeholder="common.search.placeholder"
          :aria-label="common.search.label"
          data-search-input
          autocomplete="off"
          spellcheck="false"
          @keydown.esc.stop.prevent="onEscape"
        />
        <span v-if="text !== ''" class="field-clear">
          <BaseIconButton :label="common.search.clear" @click="onClear"><Xmark class="icon" aria-hidden="true" /></BaseIconButton>
        </span>
      </div>
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

/* 入力欄の右端の中に重ねる（§7 P1: 右クリア）。ボタンは 32px（タッチ UI では 44px）で、欄の縦中央に置く */
.field-clear {
  position: absolute;
  top: 50%;
  right: var(--s-1);
  display: inline-flex;
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
  /* 狭い幅でプレースホルダが収まらないときは、字の途中で切らず「…」で省く（#348） */
  text-overflow: ellipsis;
}

/* 右のクリア（--control-h-sm）が出ているときだけ、その分を空けて文字と重ならないようにする。出ていないあいだは空けず、プレースホルダに幅を回す（#348） */
.input.has-clear {
  padding-right: calc(var(--s-1) + var(--control-h-sm) + var(--s-1));
}

/* ブラウザ標準の✕は出さない（O8）。クリアは右の自前のボタンに一本化する。 */
.input::-webkit-search-cancel-button {
  appearance: none;
}

.input:focus {
  border-color: var(--focus);
  outline: 2px solid var(--focus);
  outline-offset: -1px;
}

/* 640px 以下は 16px（iOS・Android のフォーカス時の自動ズームを避ける） */
@media (max-width: 640px) {
  .input {
    font-size: var(--font-size-base);
  }

  /* 入力した文字は 16px のまま（自動ズーム対策は入力欄の font-size で決まる）。プレースホルダだけ 14px にして、390px でも文言が収まるようにする（#348） */
  .input::placeholder {
    font-size: var(--font-size-sm);
  }
}
</style>
