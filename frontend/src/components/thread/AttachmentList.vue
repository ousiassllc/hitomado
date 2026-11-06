<script setup lang="ts">
import type { AttachmentInfo } from '../../../bindings/github.com/ousiassllc/hitomado/internal/domain/model'
import { formatSize } from '../../lib/format'
import { thread as msg } from '../../lib/messages'
import BaseButton from '../ui/BaseButton.vue'

defineProps<{ attachments: AttachmentInfo[] }>()
const emit = defineEmits<{ save: [attachment: AttachmentInfo] }>()
</script>

<template>
  <section v-if="attachments.length > 0" class="attachments" :aria-label="msg.attachments">
    <ul>
      <li v-for="a in attachments" :key="a.id" class="attachment" data-testid="attachment">
        <span class="attachment-name" :title="a.filename">{{ a.filename }}</span>
        <span class="attachment-size">{{ formatSize(a.size) }}</span>
        <BaseButton size="sm" :aria-label="msg.saveAttachmentLabel(a.filename)" @click="emit('save', a)">
          {{ msg.saveAttachment }}
        </BaseButton>
      </li>
    </ul>
  </section>
</template>

<style scoped>
ul {
  display: flex;
  flex-wrap: wrap;
  gap: var(--s-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.attachment {
  display: flex;
  align-items: center;
  gap: var(--s-2);
  padding: var(--s-1) var(--s-2);
  border: 1px solid var(--border);
  border-radius: var(--r-sm);
  background: var(--bone);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  max-width: 100%;
}

.attachment-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 28ch;
  color: var(--ink);
}

.attachment-size {
  color: var(--mute);
  font-family: var(--font-mono);
  white-space: nowrap;
}
</style>
