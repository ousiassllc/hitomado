<script setup lang="ts">
import { common } from '../../lib/messages'

const props = defineProps<{ label: string; value: number; min: number; max: number }>()
const emit = defineEmits<{ resize: [delta: number] }>()

const STEP = 16
let startX = 0

function onPointerDown(ev: PointerEvent): void {
  startX = ev.clientX
  const target = ev.currentTarget as HTMLElement // pointer capture の対象要素
  target.setPointerCapture(ev.pointerId)
  const move = (e: PointerEvent) => {
    emit('resize', e.clientX - startX)
    startX = e.clientX
  }
  const up = () => {
    target.removeEventListener('pointermove', move)
    target.removeEventListener('pointerup', up)
    target.removeEventListener('pointercancel', up)
  }
  target.addEventListener('pointermove', move)
  target.addEventListener('pointerup', up)
  target.addEventListener('pointercancel', up)
}

function onKeydown(ev: KeyboardEvent): void {
  if (ev.key === 'ArrowLeft' || ev.key === 'ArrowRight') {
    ev.preventDefault()
    emit('resize', ev.key === 'ArrowRight' ? STEP : -STEP)
  }
}
</script>

<template>
  <hr
    class="splitter"
    aria-orientation="vertical"
    tabindex="0"
    :aria-label="`${label}（${common.shell.resizeHint}）`"
    :aria-valuenow="props.value"
    :aria-valuemin="props.min"
    :aria-valuemax="props.max"
    @pointerdown.prevent="onPointerDown"
    @keydown="onKeydown"
  />
</template>

<style scoped>
.splitter {
  flex: none;
  width: 5px;
  height: auto;
  margin: 0;
  border: 0;
  cursor: col-resize;
  background: transparent;
  z-index: var(--z-sticky);
  touch-action: none;
}

.splitter:hover,
.splitter:focus-visible {
  background: var(--accent);
  outline: none;
}
</style>
