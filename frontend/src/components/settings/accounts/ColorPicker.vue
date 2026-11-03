<script setup lang="ts">
import { useId } from 'vue'
import { settings as msg } from '../../../lib/messages'
import { ACCOUNT_COLOR_KEYS, accountColorVar, accountTextVar } from '../../../lib/palette'

defineProps<{ modelValue: string; disabled?: boolean }>()
defineEmits<{ 'update:modelValue': [key: string] }>()

// ラジオ名はピッカーごとに一意にする（アカウント行が複数並ぶため）。矢印キーでの移動はネイティブ。
const group = useId()

function colorName(key: string): string {
  return msg.accounts.colorNames[key] ?? key
}
</script>

<template>
  <fieldset class="picker" :disabled="disabled">
    <legend class="legend">{{ msg.accounts.colorLabel }}</legend>
    <div class="swatches">
      <label
        v-for="key in ACCOUNT_COLOR_KEYS"
        :key="key"
        class="swatch"
        :class="{ selected: key === modelValue }"
        :title="colorName(key)"
        :style="{ background: accountColorVar(key), color: accountTextVar(key) }"
      >
        <input
          class="radio"
          type="radio"
          :name="group"
          :value="key"
          :checked="key === modelValue"
          :aria-label="msg.accounts.colorFor(colorName(key))"
          @change="$emit('update:modelValue', key)"
        />
        <span v-if="key === modelValue" aria-hidden="true">✓</span>
      </label>
    </div>
    <span class="current">{{ msg.accounts.colorCurrent(colorName(modelValue)) }}</span>
  </fieldset>
</template>

<style scoped>
.picker {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  border: none;
}

.legend {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
}

.swatches {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
}

.swatch {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 2px solid transparent;
  border-radius: var(--radius-sm);
  font-weight: 700;
  line-height: 1;
  cursor: pointer;
}

.swatch.selected {
  border-color: var(--color-text);
}

.swatch:has(.radio:focus-visible) {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
}

.radio {
  position: absolute;
  inset: 0;
  margin: 0;
  opacity: 0;
  cursor: pointer;
}

.picker:disabled .swatch {
  opacity: 0.55;
  cursor: not-allowed;
}

.current {
  color: var(--color-text-muted);
  font-size: var(--font-size-xs);
}
</style>
