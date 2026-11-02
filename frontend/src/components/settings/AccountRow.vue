<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { Account } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { settings as msg } from '../../lib/messages'
import AccountChip from '../account/AccountChip.vue'
import BaseButton from '../ui/BaseButton.vue'
import BaseIconButton from '../ui/BaseIconButton.vue'
import BaseSwitch from '../ui/BaseSwitch.vue'
import ColorPicker from './ColorPicker.vue'
import SettingsInput from './SettingsInput.vue'

const props = defineProps<{
  account: Account
  /** 0 始まりの表示位置 */
  index: number
  total: number
  /** 'active' | 'syncing' | 'needs_reauth' | 'error'（一時状態を含む） */
  status: string
  busy: boolean
}>()

const emit = defineEmits<{
  rename: [name: string]
  color: [key: string]
  notify: [enabled: boolean]
  move: [direction: -1 | 1]
  reconnect: []
  remove: []
}>()

const draft = ref(props.account.displayName)
watch(
  () => props.account.displayName,
  (v) => {
    draft.value = v
  },
)

const trimmed = computed(() => draft.value.trim())
const dirty = computed(() => trimmed.value !== props.account.displayName)
const nameError = computed(() => (trimmed.value === '' ? msg.accounts.displayNameRequired : ''))

function submitName(): void {
  if (!dirty.value || nameError.value) return
  emit('rename', trimmed.value)
}

const statusText = computed(() => {
  if (props.status === 'needs_reauth') return msg.accounts.needsReauth
  if (props.status === 'error') return msg.accounts.statusError
  return ''
})
</script>

<template>
  <li class="row" :data-account-id="account.id">
    <div class="head">
      <AccountChip
        :account="{ displayName: account.displayName, email: account.email, color: account.color }"
        :show-email="true"
      />
      <span v-if="statusText" class="status" role="status">{{ statusText }}</span>
      <span class="spacer" />
      <BaseButton v-if="status === 'needs_reauth'" :loading="busy" @click="emit('reconnect')">
        {{ busy ? msg.accounts.reconnecting : msg.accounts.reconnect }}
      </BaseButton>
      <BaseIconButton
        :label="msg.accounts.moveUp(account.displayName)"
        :disabled="busy || index === 0"
        :data-move="`${account.id}:up`"
        @click="emit('move', -1)"
      >
        <span aria-hidden="true">↑</span>
      </BaseIconButton>
      <BaseIconButton
        :label="msg.accounts.moveDown(account.displayName)"
        :disabled="busy || index === total - 1"
        :data-move="`${account.id}:down`"
        @click="emit('move', 1)"
      >
        <span aria-hidden="true">↓</span>
      </BaseIconButton>
    </div>

    <div class="name">
      <SettingsInput
        v-model="draft"
        :label="msg.accounts.displayName"
        :error="dirty ? nameError : ''"
        @enter="submitName"
      />
      <BaseButton :disabled="!dirty || !!nameError || busy" @click="submitName">
        {{ msg.save }}
      </BaseButton>
    </div>

    <ColorPicker
      :model-value="account.color"
      :disabled="busy"
      @update:model-value="emit('color', $event)"
    />

    <div class="foot">
      <span class="notify">
        <BaseSwitch
          :model-value="account.notifyEnabled"
          :ariaLabel="msg.accounts.notifyFor(account.displayName)"
          @update:model-value="emit('notify', $event)"
        />
        <span>{{ msg.accounts.notify }}：{{ account.notifyEnabled ? msg.accounts.notifyOn : msg.accounts.notifyOff }}</span>
      </span>
      <BaseButton variant="danger" :disabled="busy" @click="emit('remove')">
        {{ msg.accounts.removeFor(account.displayName) }}
      </BaseButton>
    </div>
  </li>
</template>

<style scoped>
.row {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-bg-subtle);
  list-style: none;
}

.head,
.foot,
.notify {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.foot {
  justify-content: space-between;
}

.spacer {
  flex: 1;
}

.status {
  color: var(--color-warning-text);
  background: var(--color-warning-bg);
  padding: 0 var(--space-2);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-xs);
}

.name {
  display: flex;
  align-items: flex-end;
  gap: var(--space-2);
}

.name :deep(.field) {
  flex: 1;
}
</style>
