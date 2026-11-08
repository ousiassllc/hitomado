<script setup lang="ts">
import { Xmark } from '@iconoir/vue'
import {
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
} from 'reka-ui'
import { computed, onBeforeUnmount, watch } from 'vue'
import { common } from '../../lib/messages'
import { vRipple } from '../../lib/ripple'
import { useUiStore } from '../../stores/ui'
import AlertStack from '../shell/AlertStack.vue'

// 外側（スクラム）クリック・Esc・× は、どれも「キャンセル」として `update:open(false)` を出す（ガイド O1）。
// 確認を迫るダイアログは、呼び出し側が `update:open(false)` を「キャンセル」と同じ処理につなぐ。
// 送信中・保存中のように中断できない間は `busy` で、「キャンセル」の無い確認（OpenAI の確認など）は `closable=false` で、閉じる操作を無効にする。
const props = withDefaults(
  defineProps<{
    open: boolean
    title: string
    description?: string
    /** 中断できない処理の間（送信中・保存中）。外側クリック・Esc・× で閉じない。 */
    busy?: boolean
    /** false なら、外側クリック・Esc・× で閉じない（明示のボタンだけで閉じる確認）。 */
    closable?: boolean
    size?: 'sm' | 'md' | 'lg'
    /** 本文にフォーカスできる要素が無いとき（テキストだけの確認など）の領域名。指定すると本文が Tab で入れる region になり、キーボードでスクロールできる（ガイド B6）。 */
    bodyLabel?: string
  }>(),
  { description: undefined, busy: false, closable: true, size: 'md', bodyLabel: undefined },
)

const emit = defineEmits<{ 'update:open': [open: boolean] }>()

// 開いている間は最前面のダイアログとして登録し、失敗のアラートをダイアログの中に出す（外のアラートはスクラムの裏に隠れるため）。
const ui = useUiStore()
const mark = Symbol('dialog')
watch(
  () => props.open,
  (open) => (open ? ui.enterOverlay(mark) : ui.leaveOverlay(mark)),
  { immediate: true },
)
onBeforeUnmount(() => ui.leaveOverlay(mark))

const locked = computed(() => props.busy || !props.closable)

function onUpdate(v: boolean): void {
  if (!v && locked.value) return
  emit('update:open', v)
}

function guard(e: Event): void {
  if (locked.value) e.preventDefault()
}
</script>

<template>
  <DialogRoot :open="open" @update:open="onUpdate">
    <DialogPortal>
      <DialogOverlay class="dlg-overlay" />
      <DialogContent
        class="dlg"
        :class="`dlg-${size}`"
        :aria-describedby="description ? undefined : ''"
        @escape-key-down="guard"
        @pointer-down-outside="guard"
        @interact-outside="guard"
      >
        <header class="dlg-head">
          <DialogTitle class="dlg-title">{{ title }}</DialogTitle>
          <DialogClose v-if="!locked" v-ripple class="dlg-close" :aria-label="common.close"><Xmark class="icon" aria-hidden="true" /></DialogClose>
        </header>
        <DialogDescription v-if="description" class="dlg-desc">{{ description }}</DialogDescription>
        <AlertStack v-if="ui.isTopOverlay(mark)" class="dlg-alerts" />
        <div
          class="dlg-body"
          :tabindex="bodyLabel ? 0 : undefined"
          :role="bodyLabel ? 'region' : undefined"
          :aria-label="bodyLabel"
        >
          <slot />
        </div>
        <footer v-if="$slots.footer" class="dlg-foot"><slot name="footer" /></footer>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<style scoped>
.dlg-overlay {
  position: fixed;
  inset: 0;
  background: var(--scrim);
  z-index: var(--z-modal);
}

/* 出現はフェードイン、退出はフェードアウト（スクラムもダイアログと一緒にフェード。ガイド M5）。 */
.dlg-overlay[data-state='open'] {
  animation: ui-fade-in var(--motion-fade-in) var(--ease-out);
}

.dlg-overlay[data-state='closed'] {
  animation: ui-fade-out var(--motion-fade-out) var(--ease-in) forwards;
}

.dlg {
  position: fixed;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  display: flex;
  flex-direction: column;
  max-height: calc(100vh - var(--s-12));
  width: min(var(--dlg-w, 520px), calc(100vw - var(--s-8)));
  padding: var(--pad-card-sm);
  background: var(--surface);
  color: var(--ink);
  border: 1px solid var(--border);
  border-radius: var(--r-xl);
  box-shadow: var(--shadow-modal);
  z-index: var(--z-modal);
}

.dlg[data-state='open'] {
  animation: ui-rise-in-centered var(--motion-fade-in) var(--ease-out);
}

.dlg[data-state='closed'] {
  animation: ui-sink-out-centered var(--motion-fade-out) var(--ease-in) forwards;
}

.dlg-sm {
  --dlg-w: 400px;
}

.dlg-md {
  --dlg-w: 560px;
}

.dlg-lg {
  --dlg-w: 840px;
}

.dlg-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--s-3);
}

.dlg-title {
  margin: 0;
  font-size: var(--font-size-lg);
  line-height: var(--line-height-lg);
  font-weight: 600;
}

.dlg-close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: var(--control-h-sm);
  height: var(--control-h-sm);
  border: none;
  border-radius: var(--r-md);
  background: transparent;
  color: var(--mute);
  line-height: 1;
  cursor: pointer;
}

.dlg-close:hover {
  background: var(--accent-soft);
}

.dlg-desc {
  margin: var(--s-2) 0 0;
  color: var(--mute);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.dlg-alerts {
  margin-top: var(--s-3);
}

/* 本文は縦にスクロールする（overflow: auto）ので、子のフォーカスリング（外側 offset 2 + 2px = 4px）が縁で切れる。
   --s-1 の余白を四辺に足し、同じ分だけ margin で打ち消して見た目の位置は変えない。 */
.dlg-body {
  margin: calc(var(--s-3) - var(--s-1)) calc(var(--s-1) * -1) calc(var(--s-1) * -1);
  padding: var(--s-1);
  overflow: auto;
  min-height: 0;
}

.dlg-foot {
  display: flex;
  justify-content: flex-end;
  gap: var(--gap-inline);
  margin-top: var(--gap-field);
}
</style>
