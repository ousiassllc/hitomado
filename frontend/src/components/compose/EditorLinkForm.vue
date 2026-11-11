<script setup lang="ts">
import { nextTick, onMounted, ref } from 'vue'
import { compose as msg } from '../../lib/messages'
import { normalizeLinkUrl } from '../../lib/richtext'
import BaseButton from '../ui/BaseButton.vue'

const props = defineProps<{ initial: string }>()
const emit = defineEmits<{ apply: [href: string]; remove: []; cancel: [] }>()

const url = ref(props.initial)
const invalid = ref(false)
const input = ref<HTMLInputElement>()

onMounted(() => void nextTick(() => input.value?.focus()))

function submit(): void {
  if (url.value.trim() === '') {
    emit('remove')
    return
  }
  const href = normalizeLinkUrl(url.value)
  if (href === undefined) {
    invalid.value = true
    return
  }
  emit('apply', href)
}
</script>

<template>
  <form class="rte-link" @submit.prevent="submit" @keydown.esc.stop="emit('cancel')">
    <input
      ref="input"
      v-model="url"
      class="rte-link-input"
      type="text"
      inputmode="url"
      :aria-label="msg.editor.linkUrl"
      :aria-invalid="invalid"
      :placeholder="msg.editor.linkUrl"
      @input="invalid = false"
    />
    <BaseButton size="sm" variant="primary" type="submit">{{ msg.editor.linkApply }}</BaseButton>
    <BaseButton size="sm" @click="emit('remove')">{{ msg.editor.linkRemove }}</BaseButton>
    <BaseButton size="sm" variant="ghost" @click="emit('cancel')">{{ msg.editor.linkCancel }}</BaseButton>
    <span v-if="invalid" class="rte-link-error" role="alert">{{ msg.editor.linkInvalid }}</span>
  </form>
</template>

<style scoped>
.rte-link {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-2);
  padding: var(--s-2);
  border-bottom: 1px solid var(--border);
  background: var(--bone);
}

.rte-link-input {
  flex: 1 1 240px;
  min-width: 0;
  height: var(--control-h);
  padding: 0 var(--s-3);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  color: var(--ink);
  font-size: var(--font-size-sm);
}

.rte-link-input:focus {
  border-color: var(--focus);
  outline: 2px solid var(--focus);
  outline-offset: -1px;
}

/* 640px 以下は 16px（iOS・Android のフォーカス時の自動ズームを避ける） */
@media (max-width: 640px) {
  .rte-link-input {
    font-size: var(--font-size-base);
  }
}

.rte-link-input[aria-invalid="true"] {
  border-color: var(--danger);
}

.rte-link-error {
  flex-basis: 100%;
  color: var(--danger);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}
</style>
