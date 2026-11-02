<script setup lang="ts">
import { Dialogs } from '@wailsio/runtime'
import { computed, onBeforeUnmount, onMounted } from 'vue'
import { droppedPaths, FILES_DROPPED_EVENT } from '../../lib/dropFiles'
import { onEvent } from '../../lib/events'
import { formatSize } from '../../lib/format'
import { compose as msg } from '../../lib/messages'
import { useComposeStore } from '../../stores/compose'
import { useUiStore } from '../../stores/ui'
import BaseButton from '../ui/BaseButton.vue'

/** Gmail の添付合計上限（超過の判定は Go。表示用）。 */
const LIMIT_BYTES = 25 * 1024 * 1024

const compose = useComposeStore()
const ui = useUiStore()

const total = computed(() => compose.attachments.reduce((sum, a) => sum + a.size, 0))
const disabled = computed(() => compose.sending || compose.preview !== undefined)

async function pick(): Promise<void> {
  try {
    const picked = await Dialogs.OpenFile({
      Title: msg.attachments.add,
      CanChooseFiles: true,
      CanChooseDirectories: false,
      AllowsMultipleSelection: true,
    })
    const paths = (Array.isArray(picked) ? picked : [picked]).filter((p) => p !== '')
    if (paths.length > 0) await compose.addAttachments(paths)
  } catch {
    // 選択のキャンセルはエラーにしない。ダイアログを開けなかった場合のみ知らせる。
    ui.pushToast('error', msg.attachments.pickFailed)
  }
}

let stopDrop: (() => void) | undefined
onMounted(() => {
  // Wails はドロップ先（data-file-drop-target 要素）が決まったとき、Go 側が files-dropped を発行する
  stopDrop = onEvent<unknown>(FILES_DROPPED_EVENT, (data) => {
    if (disabled.value) return
    const paths = droppedPaths(data)
    if (paths.length > 0) void compose.addAttachments(paths)
  })
})
onBeforeUnmount(() => stopDrop?.())
</script>

<template>
  <section class="attachments" :aria-label="msg.attachments.label">
    <div class="attach-head">
      <BaseButton variant="secondary" :disabled="disabled" @click="pick">📎 {{ msg.attachments.add }}</BaseButton>
      <span v-if="compose.attachments.length > 0" class="attach-total">
        {{ msg.attachments.total(formatSize(total)) }} / {{ msg.attachments.limit(formatSize(LIMIT_BYTES)) }}
      </span>
      <span v-else class="attach-hint">{{ msg.attachments.dropHint }}</span>
    </div>
    <p v-if="compose.attachmentError" class="attach-error" role="alert">{{ compose.attachmentError }}</p>
    <ul v-if="compose.attachments.length > 0" class="attach-list">
      <li v-for="a in compose.attachments" :key="a.id" class="attach-item">
        <span class="attach-name" :title="a.filename">{{ a.filename }}</span>
        <span class="attach-size">{{ formatSize(a.size) }}</span>
        <span v-if="a.source === 'forwarded'" class="attach-tag">{{ msg.attachments.forwarded }}</span>
        <button
          type="button"
          class="attach-remove"
          :aria-label="msg.attachments.remove(a.filename)"
          :disabled="disabled"
          @click="compose.removeAttachment(a.id)"
        >
          ×
        </button>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.attachments {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.attach-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-3);
}

.attach-total,
.attach-hint {
  color: var(--color-text-muted);
  font-size: var(--font-size-xs);
}

.attach-error {
  margin: 0;
  color: var(--color-danger);
  font-size: var(--font-size-sm);
}

.attach-list {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.attach-item {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  max-width: 100%;
  padding: 2px var(--space-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-bg-subtle);
  font-size: var(--font-size-sm);
}

.attach-name {
  overflow: hidden;
  max-width: 260px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.attach-size,
.attach-tag {
  color: var(--color-text-muted);
  font-size: var(--font-size-xs);
}

.attach-remove {
  width: 24px;
  height: 24px;
  padding: 0;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: inherit;
  cursor: pointer;
}

.attach-remove:hover:not(:disabled) {
  background: var(--color-selected);
}
</style>
