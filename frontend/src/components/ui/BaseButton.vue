<script setup lang="ts">
import { vRipple } from '../../lib/ripple'
import Spinner from './Spinner.vue'

withDefaults(
  defineProps<{
    variant?: 'primary' | 'secondary' | 'danger' | 'ghost'
    /** md = 高さ 40・16px。sm = 高さ 32・14px（ツールバーや行の中の小さい操作） */
    size?: 'md' | 'sm'
    loading?: boolean
    disabled?: boolean
    type?: 'button' | 'submit' | 'reset'
  }>(),
  { variant: 'secondary', size: 'md', loading: false, disabled: false, type: 'button' },
)
</script>

<template>
  <button
    v-ripple
    class="btn"
    :class="[`btn-${variant}`, `btn-${size}`]"
    :type="type"
    :disabled="disabled || loading"
    :aria-busy="loading || undefined"
  >
    <Spinner v-if="loading" />
    <slot />
  </button>
</template>

<style scoped>
.btn {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--s-2);
  min-height: var(--control-h);
  padding: 0 var(--s-4);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: transparent;
  color: var(--ink);
  font-size: var(--font-size-base);
  font-weight: 600;
  cursor: pointer;
  /* ホバーは色・背景だけ（180ms） */
  transition:
    background-color var(--motion-base) var(--ease-out),
    border-color var(--motion-base) var(--ease-out);
  white-space: nowrap;
}

.btn-sm {
  min-height: var(--control-h-sm);
  padding: 0 var(--s-3);
  font-size: var(--font-size-sm);
}

.btn:hover:not(:disabled) {
  background: var(--accent-soft);
}

/* 無効（押せない）ボタンだけを薄くする（opacity .4）。送信中など loading のボタンは薄くせず、スピナーで処理中を示す。 */
.btn:disabled:not([aria-busy='true']) {
  opacity: 0.4;
  cursor: not-allowed;
}

/* loading は寸法を変えない（opacity も掛けない）。ラベルは透明にして幅だけ残し、スピナーを中央に重ねる。 */
.btn[aria-busy='true'] {
  color: transparent;
  cursor: progress;
}

/* 中央寄せは inset + margin: auto（transform は Spinner の回転が使うので使わない）。絶対配置なので高さも幅も増えない。
   文字色が透明なので、先端はバリアントの通常の文字色、track は同じ色の 25%。 */
.btn .spinner {
  position: absolute;
  inset: 0;
  margin: auto;
  border-color: color-mix(in srgb, var(--ink) 25%, transparent);
  border-top-color: var(--ink);
}

.btn-primary {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--bone);
}

/* 地が --accent なので、先端は文字色と同じ --bone（Spinner の既定の --accent だと溶けて見えない）。 */
.btn-primary .spinner {
  border-color: color-mix(in srgb, var(--bone) 25%, transparent);
  border-top-color: var(--bone);
}

.btn-primary:hover:not(:disabled) {
  background: color-mix(in srgb, var(--accent) 85%, var(--bone));
}

.btn-danger {
  background: transparent;
  border-color: var(--danger-border);
  color: var(--danger);
}

.btn-danger:hover:not(:disabled) {
  background: var(--danger-soft);
}

.btn-danger .spinner {
  border-color: color-mix(in srgb, var(--danger) 25%, transparent);
  border-top-color: var(--danger);
}

.btn-ghost {
  background: transparent;
  border-color: transparent;
}

/* 強制カラーでは文字色が上書きされ透明にできず、文字がスピナーと重なる。色の強制を外して透明を保ち、スピナーは ButtonText で描く。 */
@media (forced-colors: active) {
  .btn[aria-busy='true'] {
    forced-color-adjust: none;
    background: ButtonFace;
    border-color: ButtonText;
  }

  .btn .spinner {
    border-color: color-mix(in srgb, ButtonText 25%, transparent);
    border-top-color: ButtonText;
  }
}
</style>
