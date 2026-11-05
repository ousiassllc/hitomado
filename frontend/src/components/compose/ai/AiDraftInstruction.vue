<script setup lang="ts">
import { computed, nextTick, onMounted, ref, useId, watch } from 'vue'
import { instructionLength, MAX_INSTRUCTION_CHARS } from '../../../lib/aiInstruction'
import { compose as msg } from '../../../lib/messages'

// 「AI への指示」の入力欄（F-08。#109）。内容に応じて 2〜6 行まで伸び、超えたら内部スクロールする。
// 値は呼び出し側（composeStore の作成セッション）が持つ。ここでは保存しない。
const model = defineModel<string>({ required: true })
defineProps<{ disabled?: boolean }>()

const MIN_ROWS = 2
const MAX_ROWS = 6

// 作成画面が同時に複数あっても label と textarea が混ざらないよう、id は固定にしない。
const inputId = useId()
const area = ref<HTMLTextAreaElement>()
const remaining = computed(() => MAX_INSTRUCTION_CHARS - instructionLength(model.value))

/** 行数に合わせて高さを決める（行の高さは computed style から読む）。 */
function fit(): void {
  const el = area.value
  if (!el) return
  const style = getComputedStyle(el)
  const line = Number.parseFloat(style.lineHeight) || 20
  const pad = Number.parseFloat(style.paddingTop) + Number.parseFloat(style.paddingBottom) || 0
  el.style.height = 'auto'
  const content = el.scrollHeight
  const min = line * MIN_ROWS + pad
  const max = line * MAX_ROWS + pad
  el.style.height = `${Math.min(Math.max(content, min), max)}px`
}

onMounted(fit)
watch(model, () => void nextTick(fit))
</script>

<template>
  <div class="instruction" data-testid="ai-instruction">
    <label class="instruction-label" :for="inputId">{{ msg.ai.instruction.label }}</label>
    <textarea
      :id="inputId"
      ref="area"
      v-model="model"
      class="instruction-input"
      :rows="MIN_ROWS"
      :placeholder="msg.ai.instruction.placeholder"
      :disabled="disabled"
      data-testid="ai-instruction-input"
    />
    <div class="instruction-foot">
      <span class="instruction-note">{{ msg.ai.instruction.note }}</span>
      <span
        class="instruction-count"
        :class="{ over: remaining < 0 }"
        role="status"
        data-testid="ai-instruction-remaining"
      >
        {{ msg.ai.instruction.remaining(remaining) }}
      </span>
    </div>
  </div>
</template>

<style scoped>
.instruction {
  display: flex;
  flex-direction: column;
  gap: var(--s-1);
}

.instruction-label {
  color: var(--mute);
  font-size: var(--font-size-sm);
}

.instruction-input {
  box-sizing: border-box;
  width: 100%;
  padding: var(--s-3);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  color: var(--ink);
  font: inherit;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  resize: none;
  overflow-y: auto;
}

.instruction-input:focus {
  border-color: var(--focus);
  box-shadow: 0 0 0 2px var(--ring);
  outline: none;
}

.instruction-foot {
  display: flex;
  justify-content: space-between;
  gap: var(--s-2);
  color: var(--mute);
  font-size: var(--font-size-xs);
}

.instruction-count.over {
  color: var(--danger);
}
</style>
