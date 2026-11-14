<script setup lang="ts">
import { computed } from 'vue'
import BaseChip from '../../../ui/BaseChip.vue'

// 設定の AI の状態チップ（接続済み・未接続・つなぎ直しが必要・準備中）。色だけでなく文言で伝える。
// ok／warn／wait は BaseChip の tint（薄い地と枠。文字は --ink）、off は中立（文字だけ --mute）。
const props = defineProps<{ kind: 'ok' | 'off' | 'warn' | 'wait' }>()
const TINTS = { ok: 'var(--success)', warn: 'var(--danger)', wait: 'var(--info)' } as const
const tint = computed(() => (props.kind === 'off' ? undefined : TINTS[props.kind]))
</script>

<template>
  <BaseChip class="status" :class="`status-${kind}`" :tint="tint" :data-status="kind"><slot /></BaseChip>
</template>

<style scoped>
.status {
  flex: none;
  height: 20px;
  line-height: 1;
}

.status.status-off {
  color: var(--mute);
}
</style>
