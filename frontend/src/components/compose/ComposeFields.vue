<script setup lang="ts">
import { computed, ref } from 'vue'
import { compose as msg } from '../../lib/messages'
import { useAccountsStore } from '../../stores/accounts'
import { useComposeStore } from '../../stores/compose'
import RecipientField from './RecipientField.vue'
import SenderField from './SenderField.vue'

// 作成画面の宛先・件名の入力欄（送信元・To・Cc・Bcc・件名）。ComposePane の一部。
defineProps<{
  /** 送信確認の表示中・送信中は入力できない。 */
  frozen: boolean
}>()

const compose = useComposeStore()
const accounts = useAccountsStore()
const session = computed(() => compose.session)

const ccOpen = ref(false)
const bccOpen = ref(false)
const showCc = computed(() => ccOpen.value || compose.content.cc.length > 0)
const showBcc = computed(() => bccOpen.value || compose.content.bcc.length > 0)

const toField = ref<InstanceType<typeof RecipientField>>()
const ccField = ref<InstanceType<typeof RecipientField>>()
const bccField = ref<InstanceType<typeof RecipientField>>()

/** 入力途中の宛先を確定する（送信確認を開く前に呼ぶ。確定後の content を使う）。 */
function commitPending(): void {
  for (const f of [toField, ccField, bccField]) f.value?.commitPending()
}
defineExpose({ commitPending })
</script>

<template>
  <div v-if="session" class="compose-fields">
    <SenderField
      :from="session.from ?? undefined"
      :account-id="session.accountId"
      :locked="session.senderLocked"
      :accounts="accounts.accounts"
      @select="compose.setSender"
    />
    <div class="to-row">
      <RecipientField
        ref="toField"
        v-model="compose.content.to"
        class="to-field"
        :label="msg.fields.to"
        :account-id="session.accountId"
        :disabled="frozen"
      />
      <div class="toggles">
        <button v-if="!showCc" type="button" class="toggle" @click="ccOpen = true">{{ msg.fields.showCc }}</button>
        <button v-if="!showBcc" type="button" class="toggle" @click="bccOpen = true">{{ msg.fields.showBcc }}</button>
      </div>
    </div>
    <RecipientField
      v-if="showCc"
      ref="ccField"
      v-model="compose.content.cc"
      :label="msg.fields.cc"
      :account-id="session.accountId"
      :disabled="frozen"
    />
    <RecipientField
      v-if="showBcc"
      ref="bccField"
      v-model="compose.content.bcc"
      :label="msg.fields.bcc"
      :account-id="session.accountId"
      :disabled="frozen"
    />
    <label class="subject-row">
      <span class="subject-label">{{ msg.fields.subject }}</span>
      <input
        v-model="compose.content.subject"
        class="subject-input"
        type="text"
        :placeholder="msg.fields.subjectPlaceholder"
        :disabled="frozen"
      />
    </label>
  </div>
</template>

<style scoped>
.compose-fields {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.to-row {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
}

.to-field {
  flex: 1;
  min-width: 0;
}

.toggles {
  display: flex;
  gap: var(--space-1);
  padding-top: 4px;
}

.toggle {
  padding: 2px var(--space-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-muted);
  font-size: var(--font-size-xs);
  cursor: pointer;
}

.toggle:hover {
  background: var(--color-selected);
}

.subject-row {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.subject-label {
  flex: 0 0 56px;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}

.subject-input {
  flex: 1;
  height: 34px;
  padding: 0 var(--space-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-bg);
  color: var(--color-text);
  font-size: var(--font-size-md);
}

.subject-input:focus {
  border-color: var(--color-focus);
  outline: none;
}
</style>
