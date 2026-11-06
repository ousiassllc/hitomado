<script setup lang="ts">
import { WarningTriangle, Xmark } from '@iconoir/vue'
import { computed } from 'vue'
import type { Address } from '../../../../bindings/github.com/ousiassllc/hitomado/internal/domain/model'
import { compose as msg } from '../../../lib/messages'
import { addressLabel, isLikelyEmail } from '../../../lib/recipients'
import { vRipple } from '../../../lib/ripple'

const props = defineProps<{
  address: Address
  disabled?: boolean
}>()

defineEmits<{ remove: [] }>()

const invalid = computed(() => !isLikelyEmail(props.address.email))
</script>

<template>
  <span class="chip" :class="{ 'chip-invalid': invalid }" :title="invalid ? msg.fields.invalidAddress : undefined">
    <WarningTriangle v-if="invalid" class="icon icon-sm chip-icon" aria-hidden="true" />
    <span class="chip-text">{{ addressLabel(address) }}</span>
    <button
      type="button"
      class="chip-remove"
      :aria-label="msg.fields.removeRecipient(address.email)"
      :disabled="disabled"
      @click.stop="$emit('remove')"
    >
      <span v-ripple class="chip-remove-face"><Xmark class="icon icon-sm" aria-hidden="true" /></span>
    </button>
  </span>
</template>

<style scoped>
.chip {
  display: inline-flex;
  align-items: center;
  max-width: 100%;
  border: 1px solid var(--border);
  border-radius: var(--r-full);
  background: var(--accent-soft);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.chip-invalid {
  border-color: var(--danger-border);
  background: var(--danger-soft);
  color: var(--danger);
}

.chip-icon {
  flex: none;
  margin-left: var(--s-2);
}

.chip-text {
  overflow: hidden;
  padding: 0 var(--s-1) 0 var(--s-2);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chip-remove {
  width: 22px;
  height: 22px;
  padding: 0;
  border: none;
  border-radius: var(--r-full);
  background: transparent;
  color: inherit;
  cursor: pointer;
}

.chip-remove:hover {
  background: var(--surface);
}

/* リップル（.has-ripple の overflow: hidden）はこの内側の円だけに持たせる。ボタン本体に付けると、touch.css が疑似要素で広げた当たり判定を切ってしまう。
   touch.css の ::before（位置指定。当たり判定）より DOM 順で後ろに置いて前面に出し、中央の押下でもリップルが出るようにする（.has-ripple の position: relative で、同じ層では後ろの要素が上）。 */
.chip-remove-face {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
  border-radius: var(--r-full);
}

.chip-remove:disabled .chip-remove-face {
  pointer-events: none;
}
</style>
