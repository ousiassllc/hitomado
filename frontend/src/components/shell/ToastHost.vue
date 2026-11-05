<script setup lang="ts">
import { CheckCircle, InfoCircle } from '@iconoir/vue'
import { onBeforeUnmount, watch } from 'vue'
import { vRipple } from '../../lib/ripple'
import { useUiStore } from '../../stores/ui'

// トースト: 画面下中央・ピル・1 行の短い結果。同時に 1 つ（新しいものが前を置き換える）、3 秒で消える（ガイド O3）。
// 戻せる結果（アーカイブ・ゴミ箱）は「元に戻す」つきのスナックバーにして、操作できるよう 6 秒出す。
// 失敗の理由・再試行は AlertStack（インラインアラート）に出す。
const ui = useUiStore()
const TOAST_MS = 3000
const SNACKBAR_MS = 6000
let timer: ReturnType<typeof setTimeout> | undefined

function stop(): void {
  if (timer) clearTimeout(timer)
  timer = undefined
}

function undo(id: number, run: () => void): void {
  ui.dismissToast(id)
  run()
}

watch(
  () => ui.toast?.id,
  (id) => {
    stop()
    if (id === undefined) return
    timer = setTimeout(() => ui.dismissToast(id), ui.toast?.action ? SNACKBAR_MS : TOAST_MS)
  },
  { immediate: true },
)

onBeforeUnmount(stop)
</script>

<template>
  <div class="toasts">
    <Transition name="toast">
      <div
        v-if="ui.toast"
        :key="ui.toast.id"
        class="toast"
        :class="{ 'toast-action': ui.toast.action }"
        role="status"
        aria-live="polite"
        :title="ui.toast.message"
      >
        <CheckCircle v-if="ui.toast.kind === 'success'" class="icon icon-sm" aria-hidden="true" />
        <InfoCircle v-else class="icon icon-sm" aria-hidden="true" />
        <span class="msg">{{ ui.toast.message }}</span>
        <button
          v-if="ui.toast.action"
          v-ripple
          class="undo"
          type="button"
          data-testid="snackbar-action"
          @click="undo(ui.toast.id, ui.toast.action.run)"
        >
          {{ ui.toast.action.label }}
        </button>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.toasts {
  position: fixed;
  left: 50%;
  bottom: var(--toast-offset);
  transform: translateX(-50%);
  display: flex;
  justify-content: center;
  max-width: calc(100vw - var(--s-8));
  pointer-events: none;
  z-index: var(--z-toast);
}

.toast {
  display: flex;
  align-items: center;
  gap: var(--s-2);
  height: var(--toast-h);
  padding: 0 var(--s-4);
  border-radius: var(--r-full);
  background: var(--ink);
  color: var(--bone);
  font-size: var(--font-size-sm);
  line-height: 1;
}

.toast-action {
  pointer-events: auto;
}

/* 「元に戻す」: ink 地の上の bone 字のテキストボタン（素のボタンに v-ripple。ガイドの小さな操作と同じ作り） */
.undo {
  flex: none;
  min-height: var(--control-h-sm);
  margin-right: calc(var(--s-2) * -1);
  padding: 0 var(--s-3);
  border: 0;
  border-radius: var(--r-full);
  background: transparent;
  color: var(--bone);
  font: inherit;
  font-weight: 600;
  cursor: pointer;
  text-decoration: underline;
  transition: background-color var(--motion-base) var(--ease-out);
}

.undo:hover {
  background: color-mix(in srgb, var(--bone) 16%, transparent);
}

.msg {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.toast-enter-active {
  animation: ui-rise-in var(--motion-fade-in) var(--ease-out);
}

.toast-leave-active {
  animation: ui-sink-out var(--motion-fade-out) var(--ease-in) forwards;
}
</style>
