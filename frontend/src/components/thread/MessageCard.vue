<script setup lang="ts">
import { ref, watch } from 'vue'
import type { ThreadMessage } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import type { AttachmentInfo } from '../../../bindings/github.com/ousiassllc/hitomado/internal/domain/model'
import { formatDateTime } from '../../lib/format'
import { thread as msg } from '../../lib/messages'
import AttachmentList from './AttachmentList.vue'
import MessageBody from './MessageBody.vue'
import { formatAddress, formatAddresses, shortName } from './threadView'

const props = defineProps<{ message: ThreadMessage; expanded: boolean }>()
const emit = defineEmits<{ link: [url: string]; saveAttachment: [attachment: AttachmentInfo] }>()

/** 開閉はカード内で切り替える。親の既定（expanded）が変わったときだけそれに合わせる。 */
const open = ref(props.expanded)
watch(
  () => props.expanded,
  (v) => {
    open.value = v
  },
)
</script>

<template>
  <article class="message-card" :class="{ 'is-unread': message.isUnread, 'is-open': open }" :data-message-id="message.id">
    <button
      type="button"
      class="card-toggle"
      data-testid="card-toggle"
      :aria-expanded="open"
      :aria-label="open ? msg.collapse : msg.expand"
      @click="open = !open"
    >
      <span class="toggle-mark" aria-hidden="true">{{ open ? '▾' : '▸' }}</span>
      <span class="sender" :title="formatAddress(message.from)">{{ shortName(message.from) }}</span>
      <span v-if="message.isUnread" class="unread-badge">{{ msg.unreadMark }}</span>
      <span v-if="!open && message.attachments?.length" class="clip">{{ msg.attachmentCount(message.attachments.length) }}</span>
      <span class="date">{{ formatDateTime(message.date) }}</span>
    </button>

    <div v-if="open" class="card-content" data-testid="card-content">
      <dl class="headers">
        <div>
          <dt>{{ msg.from }}</dt>
          <dd>{{ formatAddress(message.from) }}</dd>
        </div>
        <div>
          <dt>{{ msg.to }}</dt>
          <dd>{{ message.to?.length ? formatAddresses(message.to) : msg.toMe }}</dd>
        </div>
        <div v-if="message.cc?.length">
          <dt>{{ msg.cc }}</dt>
          <dd>{{ formatAddresses(message.cc) }}</dd>
        </div>
      </dl>
      <MessageBody
        :html="message.bodyHtml"
        :text="message.bodyText"
        :has-remote-images="message.hasRemoteImages"
        @link="emit('link', $event)"
      />
      <AttachmentList :attachments="message.attachments ?? []" @save="emit('saveAttachment', $event)" />
    </div>
  </article>
</template>

<style scoped>
.message-card {
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-bg);
}

.message-card.is-unread {
  background: var(--color-unread-bg);
}

.card-toggle {
  display: flex;
  align-items: baseline;
  gap: var(--space-2);
  width: 100%;
  padding: var(--space-2) var(--space-3);
  border: 0;
  background: transparent;
  color: var(--color-text);
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.card-toggle:focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: -2px;
}

.toggle-mark {
  color: var(--color-text-muted);
  width: 1em;
}

.sender {
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

.unread-badge {
  padding: 0 var(--space-2);
  border-radius: var(--radius-sm);
  background: var(--color-accent);
  color: var(--color-accent-text);
  font-size: var(--font-size-xs);
}

.clip {
  color: var(--color-text-muted);
  font-size: var(--font-size-xs);
}

.date {
  margin-left: auto;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
  white-space: nowrap;
}

.card-content {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: 0 var(--space-3) var(--space-3);
}

.headers {
  margin: 0;
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
}

.headers > div {
  display: flex;
  gap: var(--space-2);
}

dt {
  flex: none;
  min-width: 3em;
}

dd {
  margin: 0;
  overflow-wrap: anywhere;
  color: var(--color-text);
}
</style>
