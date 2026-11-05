<script setup lang="ts">
import { CheckCircle, InfoCircle } from '@iconoir/vue'
import { onBeforeUnmount, watch } from 'vue'
import { useUiStore } from '../../stores/ui'

// トースト: 画面下中央・ピル・1 行の短い結果。同時に 1 つ（新しいものが前を置き換える）、3 秒で消える（ガイド O3）。
// 失敗の理由・再試行は AlertStack（インラインアラート）に出す。
const ui = useUiStore()
let timer: ReturnType<typeof setTimeout> | undefined

function stop(): void {
  if (timer) clearTimeout(timer)
  timer = undefined
}

watch(
  () => ui.toast?.id,
  (id) => {
    stop()
    if (id === undefined) return
    timer = setTimeout(() => ui.dismissToast(id), 3000)
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
        role="status"
        aria-live="polite"
        :title="ui.toast.message"
      >
        <CheckCircle v-if="ui.toast.kind === 'success'" class="icon icon-sm" aria-hidden="true" />
        <InfoCircle v-else class="icon icon-sm" aria-hidden="true" />
        <span class="msg">{{ ui.toast.message }}</span>
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
