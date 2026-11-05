<script setup lang="ts">
import { compose as msg } from '../../../lib/messages'
import BaseButton from '../../ui/BaseButton.vue'
import BaseDialog from '../../ui/BaseDialog.vue'

// AI の設定がまだ無いときに「AI で下書き」を押した場合の初回の案内（#104）。選んだ道は呼び出し側が設定画面の AI の流れへつなぐ。
defineProps<{ open: boolean }>()
const emit = defineEmits<{ choose: [choice?: 'chatgpt' | 'other'] }>()
</script>

<template>
  <BaseDialog :open="open" :title="msg.ai.setup.title" size="sm" @update:open="(v) => !v && emit('choose')">
    <div class="setup" data-testid="ai-setup">
      <p class="text">{{ msg.ai.setup.body }}</p>
      <p class="note">{{ msg.ai.setup.note }}</p>
    </div>
    <template #footer>
      <BaseButton variant="ghost" data-testid="ai-setup-later" @click="emit('choose')">
        {{ msg.ai.setup.cancel }}
      </BaseButton>
      <BaseButton variant="secondary" data-testid="ai-setup-other" @click="emit('choose', 'other')">
        {{ msg.ai.setup.other }}
      </BaseButton>
      <BaseButton variant="primary" data-testid="ai-setup-chatgpt" @click="emit('choose', 'chatgpt')">
        {{ msg.ai.setup.chatgpt }}
      </BaseButton>
    </template>
  </BaseDialog>
</template>

<style scoped>
.setup {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

.text {
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-base);
}

.note {
  color: var(--mute);
  font-size: var(--font-size-xs);
}
</style>
