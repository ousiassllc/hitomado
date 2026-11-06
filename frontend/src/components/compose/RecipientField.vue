<script setup lang="ts">
import { WarningTriangle, Xmark } from '@iconoir/vue'
import { computed, ref, useId } from 'vue'
import { ComposeService } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app'
import type { Address } from '../../../bindings/github.com/ousiassllc/hitomado/internal/domain/model'
import { useContactSuggest } from '../../composables/useContactSuggest'
import { call } from '../../lib/api'
import { compose as msg } from '../../lib/messages'
import { addressLabel, isLikelyEmail, mergeAddresses, parseAddresses } from '../../lib/recipients'
import { vRipple } from '../../lib/ripple'

const props = defineProps<{
  modelValue: Address[]
  /** 候補を引く送信元アカウント。未選択なら候補は出さない。 */
  accountId?: string
  label: string
  disabled?: boolean
}>()

const emit = defineEmits<{ 'update:modelValue': [value: Address[]] }>()

const text = ref('')
const input = ref<HTMLInputElement>()
const active = ref(-1)
const listId = useId()
const invalidId = useId()

const { suggestions, query, clear } = useContactSuggest({
  accountId: () => props.accountId,
  fetch: (accountId, prefix, limit) => call(ComposeService.SuggestContacts(accountId, prefix, limit)),
})

const shown = computed(() => {
  const have = new Set(props.modelValue.map((a) => a.email.toLowerCase()))
  return suggestions.value.filter((s) => !have.has(s.email.toLowerCase()))
})
const open = computed(() => shown.value.length > 0 && text.value.trim() !== '')
const hasInvalid = computed(() => props.modelValue.some((a) => !isLikelyEmail(a.email)))

function add(addrs: Address[]): void {
  if (addrs.length === 0) return
  emit('update:modelValue', mergeAddresses(props.modelValue, addrs))
}

function reset(): void {
  text.value = ''
  active.value = -1
  clear()
}

/** 入力中の文字列をチップにする。 */
function commit(): void {
  const addrs = parseAddresses(text.value)
  if (addrs.length > 0) add(addrs)
  reset()
}

/** 入力途中の文字列があればチップにする（送信前に呼ぶ）。 */
defineExpose({ commitPending: commit })

function pick(a: Address): void {
  add([a])
  reset()
  input.value?.focus()
}

function remove(index: number): void {
  emit('update:modelValue', props.modelValue.toSpliced(index, 1))
}

function onInput(): void {
  active.value = -1
  // 区切り文字が入力されたらそこまでをチップにする（貼り付けでなくても , ; 区切りで入力できる）
  if (/[,;\n]/.test(text.value)) {
    const parts = text.value.split(/[,;\n]/)
    const rest = parts.pop() ?? ''
    add(parseAddresses(parts.join(',')))
    text.value = rest
  }
  query(text.value)
}

function onPaste(e: ClipboardEvent): void {
  const pasted = e.clipboardData?.getData('text/plain') ?? ''
  const addrs = parseAddresses(pasted)
  if (addrs.length < 2 && !/[,;\n]/.test(pasted)) return // 単一アドレスは通常の入力として扱う
  e.preventDefault()
  add(parseAddresses(`${text.value} ${pasted}`.trim()))
  reset()
}

function onKeydown(e: KeyboardEvent): void {
  if (e.isComposing) return
  const n = shown.value.length
  if (e.key === 'ArrowDown' && open.value) {
    e.preventDefault()
    active.value = (active.value + 1) % n
  } else if (e.key === 'ArrowUp' && open.value) {
    e.preventDefault()
    active.value = (active.value - 1 + n) % n
  } else if (e.key === 'Enter' && !e.ctrlKey && !e.metaKey) {
    if (open.value && active.value >= 0) {
      e.preventDefault()
      pick(shown.value[active.value])
    } else if (text.value.trim() !== '') {
      e.preventDefault()
      commit()
    }
  } else if (e.key === 'Tab' && text.value.trim() !== '') {
    if (open.value && active.value >= 0) {
      e.preventDefault()
      pick(shown.value[active.value])
    } else {
      commit()
    }
  } else if (e.key === 'Escape' && open.value) {
    e.stopPropagation()
    clear()
  } else if (e.key === 'Backspace' && text.value === '' && props.modelValue.length > 0) {
    remove(props.modelValue.length - 1)
  }
}
</script>

<template>
  <div class="recipient-field">
    <span class="recipient-label">{{ label }}</span>
    <label class="recipient-box" :class="{ 'recipient-disabled': disabled }">
      <span
        v-for="(a, i) in modelValue"
        :key="a.email"
        class="chip"
        :class="{ 'chip-invalid': !isLikelyEmail(a.email) }"
        :title="isLikelyEmail(a.email) ? undefined : msg.fields.invalidAddress"
      >
        <WarningTriangle v-if="!isLikelyEmail(a.email)" class="icon icon-sm chip-icon" aria-hidden="true" />
        <span class="chip-text">{{ addressLabel(a) }}</span>
        <button
          v-ripple
          type="button"
          class="chip-remove"
          :aria-label="msg.fields.removeRecipient(a.email)"
          :disabled="disabled"
          @click.stop="remove(i)"
        >
          <Xmark class="icon icon-sm" aria-hidden="true" />
        </button>
      </span>
      <input
        ref="input"
        v-model="text"
        class="recipient-input"
        type="text"
        role="combobox"
        autocomplete="off"
        spellcheck="false"
        :disabled="disabled"
        :aria-label="msg.fields.addRecipient(label)"
        :aria-expanded="open"
        :aria-controls="listId"
        :aria-activedescendant="active >= 0 ? `${listId}-${active}` : undefined"
        aria-autocomplete="list"
        :aria-describedby="hasInvalid ? invalidId : undefined"
        @input="onInput"
        @keydown="onKeydown"
        @paste="onPaste"
        @blur="commit"
      />
      <div v-show="open" :id="listId" class="suggest" role="listbox" :aria-label="msg.fields.suggestions">
        <div
          v-for="(s, i) in shown"
          :id="`${listId}-${i}`"
          :key="s.email"
          class="suggest-item"
          :class="{ 'suggest-active': i === active }"
          role="option"
          tabindex="-1"
          :aria-selected="i === active"
          @mousedown.prevent="pick(s)"
        >
          <span class="suggest-email">{{ s.email }}</span>
          <span v-if="s.name" class="suggest-name">{{ s.name }}</span>
        </div>
      </div>
    </label>
    <p v-if="hasInvalid" :id="invalidId" class="recipient-invalid">
      <WarningTriangle class="icon icon-sm" aria-hidden="true" />{{ msg.fields.invalidAddress }}
    </p>
  </div>
</template>

<style scoped>
.recipient-field {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  gap: var(--s-2);
}

.recipient-label {
  flex: 0 0 56px;
  padding-top: var(--s-2);
  color: var(--mute);
  font-size: var(--font-size-sm);
}

.recipient-box {
  position: relative;
  display: flex;
  flex: 1;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-1);
  min-height: var(--control-h);
  padding: var(--s-1) var(--s-3);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
}

.recipient-box:focus-within {
  border-color: var(--focus);
  box-shadow: 0 0 0 2px var(--ring);
}

.recipient-disabled {
  opacity: 0.5;
}

.chip {
  display: inline-flex;
  align-items: center;
  max-width: 100%;
  border: 1px solid var(--border);
  border-radius: var(--r-full);
  background: var(--accent-soft);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.chip-invalid {
  border-color: var(--danger-border);
  background: var(--danger-soft);
  color: var(--danger);
}

.chip-icon {
  flex: none;
  margin-left: var(--s-2);
}

.recipient-invalid {
  display: flex;
  flex: 0 0 100%;
  align-items: center;
  gap: var(--s-1);
  margin: 0;
  padding-left: calc(56px + var(--s-2));
  color: var(--danger);
  font-size: var(--font-size-xs);
  line-height: var(--line-height-xs);
}

.chip-text {
  overflow: hidden;
  padding: 0 var(--s-1) 0 var(--s-2);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chip-remove {
  width: 22px;
  height: 22px;
  padding: 0;
  border: none;
  border-radius: var(--r-full);
  background: transparent;
  color: inherit;
  cursor: pointer;
}

.chip-remove:hover {
  background: var(--surface);
}

.recipient-input {
  flex: 1 1 160px;
  min-width: 120px;
  height: 28px;
  border: none;
  background: transparent;
  color: var(--ink);
  font-size: var(--font-size-sm);
  outline: none;
}

.suggest {
  position: absolute;
  top: 100%;
  right: 0;
  left: 0;
  z-index: var(--z-dropdown);
  margin: var(--s-1) 0 0;
  padding: var(--s-1);
  max-height: 240px;
  overflow-y: auto;
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
}

.suggest-item {
  display: flex;
  gap: var(--s-2);
  align-items: center;
  min-height: var(--menu-item-h);
  padding: 0 var(--menu-pad-x);
  border-radius: var(--r-sm);
  font-size: var(--font-size-sm);
  cursor: pointer;
}

.suggest-active,
.suggest-item:hover {
  background: var(--accent-soft);
}

.suggest-name {
  color: var(--mute);
}
</style>
