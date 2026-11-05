<script setup lang="ts">
import { nextTick, onMounted, ref } from 'vue'
import { compose as msg } from '../../lib/messages'
import { normalizeLinkUrl } from '../../lib/richtext'

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
    <button type="submit" class="rte-link-btn">{{ msg.editor.linkApply }}</button>
    <button type="button" class="rte-link-btn" @click="emit('remove')">{{ msg.editor.linkRemove }}</button>
    <button type="button" class="rte-link-btn" @click="emit('cancel')">{{ msg.editor.linkCancel }}</button>
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
  height: 28px;
  padding: 0 var(--s-2);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  color: var(--ink);
  font-size: var(--font-size-sm);
}

.rte-link-input[aria-invalid="true"] {
  border-color: var(--danger);
}

.rte-link-btn {
  height: 28px;
  padding: 0 var(--s-2);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  color: var(--ink);
  font-size: var(--font-size-xs);
  cursor: pointer;
}

.rte-link-error {
  flex-basis: 100%;
  color: var(--danger);
  font-size: var(--font-size-xs);
}
</style>
