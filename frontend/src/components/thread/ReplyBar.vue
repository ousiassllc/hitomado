<script setup lang="ts">
import { common, thread as msg } from '../../lib/messages'
import { SHORTCUT_KEYS } from '../../lib/shortcuts'
import BaseButton from '../ui/BaseButton.vue'
import BaseMenu from '../ui/BaseMenu.vue'
import BaseTooltip from '../ui/BaseTooltip.vue'

export type ReplyMode = 'reply' | 'reply_all' | 'forward'

const props = defineProps<{ draftId?: string }>()
const emit = defineEmits<{ open: [mode: ReplyMode]; openDraft: [] }>()

const items = [
  { key: 'reply', label: msg.reply, shortcut: SHORTCUT_KEYS.reply },
  { key: 'forward', label: msg.forward, shortcut: SHORTCUT_KEYS.forward },
]

function onSelect(key: string): void {
  if (key === 'reply' || key === 'forward') emit('open', key)
}
</script>

<template>
  <div class="reply-bar" data-testid="reply-bar">
    <div class="split">
      <BaseTooltip :text="common.shortcutHint(msg.replyAll, SHORTCUT_KEYS.replyAll)">
        <BaseButton variant="primary" data-testid="reply-all" @click="emit('open', 'reply_all')">{{ msg.replyAll }}</BaseButton>
      </BaseTooltip>
      <BaseMenu :items="items" @select="onSelect">
        <template #trigger>
          <BaseButton variant="primary" class="split-menu" data-testid="reply-menu" :aria-label="msg.replyMenu">▾</BaseButton>
        </template>
      </BaseMenu>
    </div>
    <BaseButton v-if="props.draftId" data-testid="open-draft" @click="emit('openDraft')">{{ msg.openDraft }}</BaseButton>
  </div>
</template>

<style scoped>
.reply-bar {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  border-top: 1px solid var(--color-border);
  background: var(--color-bg);
}

.split {
  display: flex;
  gap: 1px;
}

.split > :first-child {
  border-top-right-radius: 0;
  border-bottom-right-radius: 0;
}

.split-menu {
  border-top-left-radius: 0;
  border-bottom-left-radius: 0;
}
</style>
