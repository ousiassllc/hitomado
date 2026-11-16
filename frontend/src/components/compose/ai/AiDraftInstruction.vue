<script setup lang="ts">
import { computed, nextTick, onMounted, ref, useId, watch } from 'vue'
import { instructionLength, MAX_INSTRUCTION_CHARS } from '../../../lib/aiInstruction'
import { compose as msg } from '../../../lib/messages'
import { vOverlayScroll } from '../../../lib/overlayScroll'

// 「AI への指示」の入力欄（F-08。#109）。内容に応じて 2〜6 行まで伸び、超えたら内部スクロールする。
// 値は呼び出し側（composeStore の作成セッション）が持つ。ここでは保存しない。
const model = defineModel<string>({ required: true })
defineProps<{
  disabled?: boolean
  /** 指示が必須（新規作成の AI 下書き。#365）。ラベルに「必須」を足し、例文も新規作成向けにする。 */
  required?: boolean
}>()

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
// 必須の指示が空のまま押されたとき、呼び出し側が入力欄へフォーカスを戻す（#365）。
defineExpose({ focus: () => area.value?.focus() })
watch(model, () => void nextTick(fit))
</script>

<template>
  <div class="instruction" data-testid="ai-instruction">
    <label class="instruction-label" :for="inputId">{{ required ? msg.ai.instruction.labelRequired : msg.ai.instruction.label }}</label>
    <textarea
      :id="inputId"
      ref="area"
      v-overlay-scroll
      v-model="model"
      class="instruction-input"
      :rows="MIN_ROWS"
      :placeholder="required ? msg.ai.instruction.placeholderNew : msg.ai.instruction.placeholder"
      :aria-required="required || undefined"
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
        {{ msg.ai.instruction.remaining.before }}<span class="num">{{ remaining }}</span>{{ msg.ai.instruction.remaining.after }}
      </span>
    </div>
  </div>
</template>

<style scoped>
/* ラベル→入力は --gap-label、入力→補足（フッタ）は --gap-help（S2）。 */
.instruction {
  display: flex;
  flex-direction: column;
  gap: var(--gap-label);
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
  font-size: var(--font-size-base);
  line-height: var(--line-height-base);
  resize: none;
  overflow-y: auto;
}

.instruction-input:focus {
  border-color: var(--focus);
  outline: 2px solid var(--focus);
  outline-offset: -1px;
}

.instruction-input:disabled {
  opacity: 0.5;
}

.instruction-foot {
  display: flex;
  justify-content: space-between;
  gap: var(--s-2);
  margin-top: calc(var(--gap-help) - var(--gap-label));
  color: var(--mute);
  font-size: var(--font-size-xs);
  line-height: var(--line-height-xs);
}

.instruction-count .num {
  font-family: var(--font-mono);
}

.instruction-count.over {
  color: var(--danger);
}
</style>
