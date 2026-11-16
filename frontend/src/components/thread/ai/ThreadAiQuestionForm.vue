<script setup lang="ts">
import { computed, nextTick, onMounted, ref, useId, watch } from 'vue'
import { thread as msg } from '../../../lib/messages'
import { vOverlayScroll } from '../../../lib/overlayScroll'
import { MAX_QUESTION_CHARS, questionLength } from '../../../lib/threadAi'
import BaseButton from '../../ui/BaseButton.vue'

// スレッドへの質問の入力欄（F-18。#207）。2〜6 行で伸び、2,000 文字までの残りを出す。
// 値（書きかけ）は呼び出し側の threadAiStore.draft にだけ持つ。ここでは保存しない。
// 「質問する」ボタンか Ctrl+Enter（Mac は Cmd+Enter）で送る。Enter は改行（日本語入力の変換確定で誤送信しないため）。
const model = defineModel<string>({ required: true })
const props = defineProps<{ disabled?: boolean }>()
const emit = defineEmits<{ submit: [] }>()

const MIN_ROWS = 2
const MAX_ROWS = 6

const inputId = useId()
const area = ref<HTMLTextAreaElement>()
const remaining = computed(() => MAX_QUESTION_CHARS - questionLength(model.value))
const canSubmit = computed(() => !props.disabled && questionLength(model.value) > 0 && remaining.value >= 0)

function fit(): void {
  const el = area.value
  if (!el) return
  const style = getComputedStyle(el)
  const line = Number.parseFloat(style.lineHeight) || 20
  const pad = Number.parseFloat(style.paddingTop) + Number.parseFloat(style.paddingBottom) || 0
  el.style.height = 'auto'
  el.style.height = `${Math.min(Math.max(el.scrollHeight, line * MIN_ROWS + pad), line * MAX_ROWS + pad)}px`
}

function submit(): void {
  if (canSubmit.value) emit('submit')
}

function onKeydown(e: KeyboardEvent): void {
  if (e.key !== 'Enter' || e.isComposing) return
  if (!(e.ctrlKey || e.metaKey)) return
  e.preventDefault()
  submit()
}

onMounted(fit)
watch(model, () => void nextTick(fit))
</script>

<template>
  <form class="question" data-testid="thread-ai-question" @submit.prevent="submit">
    <label class="question-label" :for="inputId">{{ msg.ai.question.label }}</label>
    <textarea
      :id="inputId"
      ref="area"
      v-overlay-scroll
      v-model="model"
      class="question-input"
      :rows="MIN_ROWS"
      :placeholder="msg.ai.question.placeholder"
      :disabled="disabled"
      data-testid="thread-ai-question-input"
      @keydown="onKeydown"
    />
    <div class="question-foot">
      <span class="question-note">{{ msg.ai.question.note }}</span>
      <span
        class="question-count"
        :class="{ over: remaining < 0 }"
        role="status"
        data-testid="thread-ai-question-remaining"
      >
        {{ msg.ai.question.remaining.before }}<span class="num">{{ remaining }}</span>{{ msg.ai.question.remaining.after }}
      </span>
      <BaseButton size="sm" variant="primary" type="submit" :disabled="!canSubmit" data-testid="thread-ai-ask">
        {{ msg.ai.question.submit }}
      </BaseButton>
    </div>
  </form>
</template>

<style scoped>
/* ラベル→入力は --gap-label、入力→補足（フッタ）は --gap-help（S2）。 */
.question {
  display: flex;
  flex-direction: column;
  gap: var(--gap-label);
}

.question-label {
  color: var(--mute);
  font-size: var(--font-size-sm);
}

.question-input {
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

.question-input:focus {
  border-color: var(--focus);
  outline: 2px solid var(--focus);
  outline-offset: -1px;
}

.question-input:disabled {
  opacity: 0.5;
}

/* 補足の文は 1 行に収まる幅（27em ≒ 全角 27 字）が取れなければ、文字数・ボタンと同じ行に押し込まず、自分の行に出す（#349）。
   折り返した行では、文字数とボタンを右に寄せる。 */
.question-foot {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-2);
  margin-top: calc(var(--gap-help) - var(--gap-label));
  color: var(--mute);
  font-size: var(--font-size-xs);
  line-height: var(--line-height-xs);
}

/* 自分の行でも収まらない幅では、文節の切れ目で折り返す（「保存しませ／ん。」のように語の途中で切らない）。 */
.question-note {
  flex: 1 1 27em;
  min-width: 0;
  word-break: auto-phrase;
}

.question-count {
  margin-left: auto;
  white-space: nowrap;
}

.question-count .num {
  font-family: var(--font-mono);
}

.question-count.over {
  color: var(--danger);
}
</style>
