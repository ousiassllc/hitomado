<script setup lang="ts">
import { ArrowDown, ArrowUp } from '@iconoir/vue'
import { computed, ref, watch } from 'vue'
import type { Account } from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { isClientRejectedMessage } from '../../../lib/clientRejected'
import { settings as msg } from '../../../lib/messages'
import { isScopeInsufficientMessage } from '../../../lib/scopeInsufficient'
import AccountChip from '../../account/AccountChip.vue'
import BaseButton from '../../ui/BaseButton.vue'
import BaseIconButton from '../../ui/BaseIconButton.vue'
import BaseSwitch from '../../ui/BaseSwitch.vue'
import SettingsInput from '../SettingsInput.vue'
import ColorPicker from './ColorPicker.vue'

const props = defineProps<{
  account: Account
  /** 0 始まりの表示位置 */
  index: number
  total: number
  /** 'active' | 'syncing' | 'needs_reauth' | 'error'（一時状態を含む） */
  status: string
  /** Go から届いたステータス理由（account:status の message／保存済みの statusMessage） */
  statusMessage?: string
  busy: boolean
}>()

const emit = defineEmits<{
  rename: [name: string]
  color: [key: string]
  notify: [enabled: boolean]
  move: [direction: -1 | 1]
  reconnect: []
  checkClient: []
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

const clientRejected = computed(
  () => props.status === 'needs_reauth' && isClientRejectedMessage(props.statusMessage),
)

const statusText = computed(() => {
  if (clientRejected.value) return props.statusMessage
  if (props.status === 'needs_reauth' && isScopeInsufficientMessage(props.statusMessage)) {
    return props.statusMessage
  }
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
      <BaseButton size="sm" v-if="clientRejected" :disabled="busy" @click="emit('checkClient')">
        {{ msg.accounts.checkOAuthClient }}
      </BaseButton>
      <BaseButton size="sm" v-if="status === 'needs_reauth'" :loading="busy" @click="emit('reconnect')">
        {{ busy ? msg.accounts.reconnecting : msg.accounts.reconnect }}
      </BaseButton>
      <BaseIconButton
        :label="msg.accounts.moveUp(account.displayName)"
        :disabled="busy || index === 0"
        :data-move="`${account.id}:up`"
        @click="emit('move', -1)"
      >
        <ArrowUp class="icon" aria-hidden="true" />
      </BaseIconButton>
      <BaseIconButton
        :label="msg.accounts.moveDown(account.displayName)"
        :disabled="busy || index === total - 1"
        :data-move="`${account.id}:down`"
        @click="emit('move', 1)"
      >
        <ArrowDown class="icon" aria-hidden="true" />
      </BaseIconButton>
    </div>

    <div class="name">
      <SettingsInput
        v-model="draft"
        :label="msg.accounts.displayName"
        :error="dirty ? nameError : ''"
        @enter="submitName"
      />
      <BaseButton size="sm" :disabled="!dirty || !!nameError || busy" @click="submitName">
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
      <BaseButton size="sm" variant="danger" :disabled="busy" @click="emit('remove')">
        {{ msg.accounts.removeFor(account.displayName) }}
      </BaseButton>
    </div>
  </li>
</template>

<style scoped>
.row {
  display: flex;
  flex-direction: column;
  gap: var(--s-3);
  padding: var(--s-3);
  border: 1px solid var(--border);
  border-radius: var(--r-lg);
  background: var(--bone);
  list-style: none;
}

.head,
.foot,
.notify {
  display: flex;
  align-items: center;
  gap: var(--s-2);
}

.foot {
  justify-content: space-between;
}

.spacer {
  flex: 1;
}

.status {
  border: 1px solid var(--warning);
  background: var(--warning-soft);
  color: var(--warning);
  padding: 0 var(--s-2);
  border-radius: var(--r-full);
  font-size: var(--font-size-xs);
}

.name {
  display: flex;
  align-items: flex-end;
  gap: var(--s-2);
}

.name :deep(.field) {
  flex: 1;
}
</style>
