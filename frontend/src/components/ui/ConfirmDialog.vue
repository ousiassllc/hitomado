<script setup lang="ts">
import { common } from '../../lib/messages'
import BaseButton from './BaseButton.vue'
import BaseDialog from './BaseDialog.vue'

// 破壊的操作の確認（ガイド O2: 中央モーダル。トーストで確認しない）。外側クリック・Esc・「キャンセル」で閉じ、実行中（busy）だけ閉じない。
defineProps<{
  open: boolean
  title: string
  description?: string
  /** 何が起きるかの補足 */
  detail?: string
  confirmLabel: string
  /** 実行中。確認ボタンに進行表示を出し、閉じる操作を無効にする。 */
  busy?: boolean
}>()
const emit = defineEmits<{ 'update:open': [open: boolean]; confirm: [] }>()
</script>

<template>
  <BaseDialog
    :open="open"
    size="sm"
    :title="title"
    :description="description"
    :busy="busy"
    :body-label="detail ? title : undefined"
    @update:open="emit('update:open', $event)"
  >
    <p v-if="detail" class="detail">{{ detail }}</p>
    <template #footer>
      <BaseButton :disabled="busy" @click="emit('update:open', false)">{{ common.cancel }}</BaseButton>
      <BaseButton variant="danger" :loading="busy" @click="emit('confirm')">{{ confirmLabel }}</BaseButton>
    </template>
  </BaseDialog>
</template>

<style scoped>
.detail {
  margin: 0;
  font-size: var(--font-size-base);
  line-height: var(--line-height-base);
}
</style>
