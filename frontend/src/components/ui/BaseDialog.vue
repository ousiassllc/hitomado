<script setup lang="ts">
import {
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
} from 'reka-ui'
import { common } from '../../lib/messages'
import { useLayoutStore } from '../../stores/layout'

const props = withDefaults(
  defineProps<{
    open: boolean
    title: string
    description?: string
    closable?: boolean
    size?: 'sm' | 'md' | 'lg'
  }>(),
  { description: undefined, closable: true, size: 'md' },
)

const emit = defineEmits<{ 'update:open': [open: boolean] }>()
const layout = useLayoutStore()

function onUpdate(v: boolean): void {
  if (!v && !props.closable) return
  emit('update:open', v)
}

function guard(e: Event): void {
  if (!props.closable) e.preventDefault()
}
</script>

<template>
  <DialogRoot :open="open" @update:open="onUpdate">
    <DialogPortal>
      <DialogOverlay class="dlg-overlay" />
      <DialogContent
        class="dlg"
        :class="[`dlg-${size}`, { 'dlg-full': layout.isStack }]"
        :aria-describedby="description ? undefined : ''"
        @escape-key-down="guard"
        @pointer-down-outside="guard"
        @interact-outside="guard"
      >
        <header class="dlg-head">
          <DialogTitle class="dlg-title">{{ title }}</DialogTitle>
          <DialogClose v-if="closable" class="dlg-close" :aria-label="common.close">×</DialogClose>
        </header>
        <DialogDescription v-if="description" class="dlg-desc">{{ description }}</DialogDescription>
        <div class="dlg-body"><slot /></div>
        <footer v-if="$slots.footer" class="dlg-foot"><slot name="footer" /></footer>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<style scoped>
.dlg-overlay {
  position: fixed;
  inset: 0;
  background: var(--color-overlay);
  z-index: 100;
}

.dlg {
  position: fixed;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  display: flex;
  flex-direction: column;
  max-height: calc(100vh - 48px);
  width: min(var(--dlg-w, 520px), calc(100vw - 32px));
  padding: var(--space-4);
  background: var(--color-bg-elevated);
  color: var(--color-text);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-popup);
  z-index: 101;
}

/* 1 画面ずつのレイアウト（スマホ幅）では、ダイアログを全画面にする。 */
.dlg-full {
  inset: 0;
  transform: none;
  width: auto;
  max-height: none;
  border: none;
  border-radius: 0;
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
  gap: var(--space-3);
}

.dlg-title {
  margin: 0;
  font-size: var(--font-size-lg);
  font-weight: 600;
}

.dlg-close {
  width: 28px;
  height: 28px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-muted);
  font-size: var(--font-size-xl);
  line-height: 1;
  cursor: pointer;
}

.dlg-close:hover {
  background: var(--color-selected);
}

.dlg-desc {
  margin: var(--space-2) 0 0;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}

.dlg-body {
  margin-top: var(--space-3);
  overflow: auto;
  min-height: 0;
}

.dlg-foot {
  display: flex;
  justify-content: flex-end;
  gap: var(--space-2);
  margin-top: var(--space-4);
}
</style>
