<script setup lang="ts">
import { onBeforeUnmount, watch } from 'vue'
import { common } from '../../lib/messages'
import { type Toast, useUiStore } from '../../stores/ui'
import BaseButton from '../ui/BaseButton.vue'
import BaseIconButton from '../ui/BaseIconButton.vue'

const AUTO_DISMISS_MS = { success: 5000, info: 6000, error: 10000 } as const

const ui = useUiStore()
const timers = new Map<number, ReturnType<typeof setTimeout>>()

// 再試行できるエラーは利用者が閉じるまで残す。それ以外は一定時間で消す。
watch(
  () => ui.toasts.map((t) => t.id),
  () => {
    const live = new Set(ui.toasts.map((t) => t.id))
    for (const [id, timer] of timers) {
      if (!live.has(id)) {
        clearTimeout(timer)
        timers.delete(id)
      }
    }
    for (const t of ui.toasts) {
      if (timers.has(t.id) || (t.kind === 'error' && t.retry)) continue
      timers.set(
        t.id,
        setTimeout(() => ui.dismissToast(t.id), AUTO_DISMISS_MS[t.kind]),
      )
    }
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  for (const timer of timers.values()) clearTimeout(timer)
  timers.clear()
})

function retry(t: Toast): void {
  ui.dismissToast(t.id)
  t.retry?.()
}
</script>

<template>
  <div class="toasts" aria-live="polite">
    <div
      v-for="t in ui.toasts"
      :key="t.id"
      class="toast"
      :class="`toast-${t.kind}`"
      :role="t.kind === 'error' ? 'alert' : 'status'"
    >
      <span class="msg">{{ t.message }}</span>
      <BaseButton v-if="t.retry" variant="secondary" @click="retry(t)">{{ common.retry }}</BaseButton>
      <BaseIconButton :label="common.dismiss" @click="ui.dismissToast(t.id)">×</BaseIconButton>
    </div>
  </div>
</template>

<style scoped>
.toasts {
  position: fixed;
  right: var(--space-4);
  bottom: var(--space-4);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  max-width: min(420px, calc(100vw - 32px));
  z-index: 200;
}

.toast {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-left-width: 4px;
  border-radius: var(--radius-md);
  background: var(--color-bg-elevated);
  color: var(--color-text);
  box-shadow: var(--shadow-popup);
  font-size: var(--font-size-sm);
}

.toast-success {
  border-left-color: var(--color-success);
}

.toast-error {
  border-left-color: var(--color-danger);
}

.toast-info {
  border-left-color: var(--color-accent);
}

.msg {
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
}
</style>
