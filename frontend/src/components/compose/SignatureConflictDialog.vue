<script setup lang="ts">
import { compose as msg } from '../../lib/messages'
import { useComposeStore } from '../../stores/compose'
import BaseButton from '../ui/BaseButton.vue'
import BaseDialog from '../ui/BaseDialog.vue'

// 新規作成で送信元を切り替えたとき、署名部分が編集済みなら差し替える前に確認する（F-07）。
const compose = useComposeStore()
</script>

<template>
  <BaseDialog
    :open="compose.signatureConflict !== undefined"
    :title="msg.signature.conflictTitle"
    size="sm"
    @update:open="(v) => !v && compose.resolveSignatureConflict(false)"
  >
    <p class="text">{{ msg.signature.conflictBody }}</p>
    <template #footer>
      <BaseButton variant="secondary" @click="compose.resolveSignatureConflict(false)">
        {{ msg.signature.conflictKeep }}
      </BaseButton>
      <BaseButton variant="primary" @click="compose.resolveSignatureConflict(true)">
        {{ msg.signature.conflictReplace }}
      </BaseButton>
    </template>
  </BaseDialog>
</template>

<style scoped>
.text {
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-base);
}
</style>
