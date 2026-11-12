<script setup lang="ts">
import { computed } from 'vue'
import type { ThreadDetail } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { common, thread as msg } from '../../lib/messages'
import { accountColorVar } from '../../lib/palette'
import { SHORTCUT_KEYS } from '../../lib/shortcuts'
import { useThreadStore } from '../../stores/thread'
import AccountChip from '../account/AccountChip.vue'
import BackToListButton from '../shell/BackToListButton.vue'
import BaseButton from '../ui/BaseButton.vue'
import BaseMenu from '../ui/BaseMenu.vue'
import BaseTooltip from '../ui/BaseTooltip.vue'

const props = defineProps<{
  detail: ThreadDetail
  /** 「AI で要約」を押せない（生成中・API キー未設定）。 */
  summarizeDisabled?: boolean
  summarizeHint?: string
}>()
const emit = defineEmits<{ summarize: [] }>()
const store = useThreadStore()

const labelItems = computed(() => {
  const owned = new Set((props.detail.labels ?? []).map((l) => l.id))
  const items = store.accountLabels.map((l) => ({ key: l.id, label: l.name, checked: owned.has(l.id) }))
  if (items.length > 0) return items
  return [{ key: '', label: store.labelsLoading ? msg.labelsLoading : msg.labelsEmpty, disabled: true }]
})

/** メニューが開いた経路によらず、開いた時点でアカウントのラベルを取得する。 */
function onLabelsOpen(open: boolean): void {
  if (open) void store.loadLabels()
}

function onLabel(key: string): void {
  if (key) void store.toggleLabel(key)
}

const subject = computed(() => props.detail.subject || msg.noSubject)
const bandStyle = computed(() => ({ borderTopColor: accountColorVar(props.detail.account.color) }))
</script>

<template>
  <header class="thread-header" :style="bandStyle" data-testid="thread-header">
    <div class="title-row">
      <BackToListButton />
      <h2 class="subject" data-testid="thread-subject">{{ subject }}</h2>
      <div class="actions">
        <BaseTooltip :text="common.shortcutHint(msg.archive, SHORTCUT_KEYS.archive)">
          <BaseButton size="sm" data-testid="act-archive" @click="store.archive()">{{ msg.archive }}</BaseButton>
        </BaseTooltip>
        <BaseTooltip :text="common.shortcutHint(msg.trash, SHORTCUT_KEYS.trash)">
          <BaseButton size="sm" data-testid="act-trash" @click="store.trash()">{{ msg.trash }}</BaseButton>
        </BaseTooltip>
        <BaseTooltip :text="summarizeHint ?? msg.aiSummarize">
          <BaseButton size="sm" :disabled="summarizeDisabled" data-testid="act-ai-summarize" @click="emit('summarize')">
            {{ msg.aiSummarize }}
          </BaseButton>
        </BaseTooltip>
        <BaseMenu :items="labelItems" @select="onLabel" @open="onLabelsOpen">
          <template #trigger>
            <BaseButton size="sm" data-testid="act-label">{{ msg.label }}</BaseButton>
          </template>
        </BaseMenu>
        <BaseTooltip
          :text="
            store.unread
              ? common.shortcutHint(msg.markRead, SHORTCUT_KEYS.markRead)
              : common.shortcutHint(msg.markUnread, SHORTCUT_KEYS.markUnread)
          "
        >
          <BaseButton size="sm" data-testid="act-toggle-read" @click="store.toggleRead()">
            {{ store.unread ? msg.markRead : msg.markUnread }}
          </BaseButton>
        </BaseTooltip>
      </div>
    </div>
    <div class="meta-row">
      <span class="meta-label">{{ msg.receivedAt }}</span>
      <AccountChip :account="detail.account" show-email />
      <span class="count">{{ msg.messageCount((detail.messages ?? []).length) }}</span>
    </div>
  </header>
</template>

<style scoped>
.thread-header {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
  padding: var(--s-3) var(--s-4);
  border-top: 4px solid transparent;
  border-bottom: 1px solid var(--border);
  background: var(--surface);
}

.title-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--s-3);
}

.subject {
  margin: 0;
  font-weight: 600;
  font-size: var(--font-size-lg);
  line-height: var(--line-height-lg);
  overflow-wrap: anywhere;
  color: var(--ink);
}

.actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: var(--s-2);
  flex: none;
}

.meta-row {
  display: flex;
  align-items: center;
  gap: var(--s-2);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  color: var(--mute);
}

.count {
  margin-left: auto;
}
</style>
