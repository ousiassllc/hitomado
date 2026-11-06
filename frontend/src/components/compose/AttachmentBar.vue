<script setup lang="ts">
import { Attachment, Xmark } from '@iconoir/vue'
import { Dialogs } from '@wailsio/runtime'
import { computed, onBeforeUnmount, onMounted } from 'vue'
import { droppedPaths, FILES_DROPPED_EVENT } from '../../lib/dropFiles'
import { onEvent } from '../../lib/events'
import { formatSize } from '../../lib/format'
import { compose as msg } from '../../lib/messages'
import { vRipple } from '../../lib/ripple'
import { useComposeStore } from '../../stores/compose'
import { usePlatformStore } from '../../stores/platform'
import { useUiStore } from '../../stores/ui'
import BaseButton from '../ui/BaseButton.vue'

/** Gmail の添付合計上限（超過の判定は Go。表示用）。 */
const LIMIT_BYTES = 25 * 1024 * 1024

const compose = useComposeStore()
const ui = useUiStore()
const platform = usePlatformStore()

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
    ui.pushAlert(msg.attachments.pickFailed)
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
      <BaseButton size="sm" variant="secondary" :disabled="disabled" @click="pick"><Attachment class="icon icon-sm" aria-hidden="true" /> {{ msg.attachments.add }}</BaseButton>
      <span v-if="compose.attachments.length > 0" class="attach-total">
        {{ msg.attachments.total(formatSize(total)) }} / {{ msg.attachments.limit(formatSize(LIMIT_BYTES)) }}
      </span>
      <span v-else-if="!platform.touchUi" class="attach-hint">{{ msg.attachments.dropHint }}</span>
    </div>
    <p v-if="compose.attachmentError" class="attach-error" role="alert">{{ compose.attachmentError }}</p>
    <ul v-if="compose.attachments.length > 0" class="attach-list">
      <li v-for="a in compose.attachments" :key="a.id" class="attach-item">
        <span class="attach-name" :title="a.filename">{{ a.filename }}</span>
        <span class="attach-size">{{ formatSize(a.size) }}</span>
        <span v-if="a.source === 'forwarded'" class="attach-tag">{{ msg.attachments.forwarded }}</span>
        <button
          v-ripple
          type="button"
          class="attach-remove"
          :aria-label="msg.attachments.remove(a.filename)"
          :disabled="disabled"
          @click="compose.removeAttachment(a.id)"
        >
          <Xmark class="icon icon-sm" aria-hidden="true" />
        </button>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.attachments {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

.attach-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-3);
}

.attach-total,
.attach-hint {
  color: var(--mute);
  font-size: var(--font-size-xs);
  line-height: var(--line-height-xs);
}

.attach-error {
  margin: 0;
  color: var(--danger);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.attach-list {
  display: flex;
  flex-wrap: wrap;
  gap: var(--s-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.attach-item {
  display: inline-flex;
  align-items: center;
  gap: var(--s-2);
  max-width: 100%;
  padding: var(--s-1) var(--s-2);
  border: 1px solid var(--border);
  border-radius: var(--r-sm);
  background: var(--bone);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.attach-name {
  overflow: hidden;
  max-width: 260px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.attach-size,
.attach-tag {
  color: var(--mute);
  font-size: var(--font-size-xs);
}

.attach-size {
  font-family: var(--font-mono);
}

.attach-remove {
  width: 24px;
  height: 24px;
  padding: 0;
  border: none;
  border-radius: var(--r-sm);
  background: transparent;
  color: inherit;
  cursor: pointer;
}

.attach-remove:hover:not(:disabled) {
  background: var(--accent-soft);
}

.attach-remove:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
</style>
