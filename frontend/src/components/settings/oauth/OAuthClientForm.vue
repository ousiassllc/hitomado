<script setup lang="ts">
import { ref } from 'vue'
import type { OAuthClient } from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { parseBindingError } from '../../../lib/api'
import { settings as msg } from '../../../lib/messages'
import { useAccountsStore } from '../../../stores/accounts'
import { useUiStore } from '../../../stores/ui'
import BaseButton from '../../ui/BaseButton.vue'
import SettingsInput from '../SettingsInput.vue'

const props = defineProps<{ client?: OAuthClient }>()
const emit = defineEmits<{ saved: []; cancel: [] }>()

const accountsStore = useAccountsStore()
const ui = useUiStore()

const name = ref(props.client?.name ?? '')
const clientId = ref(props.client?.clientId ?? '')
// Secret は入力後も伏字（type=password）。保存後は state ごと破棄する。編集時は未入力なら変更なし。
const secret = ref('')
const errors = ref<{ name?: string; clientId?: string; secret?: string; form?: string }>({})
const saving = ref(false)

function validate(): boolean {
  const e: typeof errors.value = {}
  if (name.value.trim() === '') e.name = msg.oauth.required(msg.oauth.name)
  if (clientId.value.trim() === '') e.clientId = msg.oauth.required(msg.oauth.clientId)
  if (!props.client && secret.value === '') e.secret = msg.oauth.required(msg.oauth.clientSecret)
  errors.value = e
  return Object.keys(e).length === 0
}

async function submit(): Promise<void> {
  if (saving.value || !validate()) return
  saving.value = true
  try {
    if (props.client) {
      await accountsStore.updateOAuthClient(
        props.client.id,
        name.value.trim(),
        clientId.value.trim(),
        secret.value === '' ? undefined : secret.value,
      )
    } else {
      await accountsStore.addOAuthClient(name.value.trim(), clientId.value.trim(), secret.value)
    }
    secret.value = ''
    ui.pushToast('success', props.client ? msg.oauth.savedEdit : msg.oauth.savedAdd)
    emit('saved')
  } catch (e) {
    // 重複 client_id などの VALIDATION もここに載せる（Go 側のメッセージをそのまま表示）。
    errors.value = { form: parseBindingError(e).message }
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <form class="form" novalidate @submit.prevent="submit">
    <h4 class="title">{{ client ? msg.oauth.formEdit : msg.oauth.formAdd }}</h4>
    <SettingsInput
      v-model="name"
      :label="msg.oauth.name"
      :placeholder="msg.oauth.namePlaceholder"
      :error="errors.name"
    />
    <SettingsInput v-model="clientId" :label="msg.oauth.clientId" :error="errors.clientId" />
    <SettingsInput
      v-model="secret"
      type="password"
      autocomplete="new-password"
      :label="msg.oauth.clientSecret"
      :placeholder="client ? msg.oauth.secretPlaceholderEdit : undefined"
      :error="errors.secret"
    />
    <p v-if="errors.form" class="form-error" role="alert">{{ errors.form }}</p>
    <div class="actions">
      <BaseButton @click="emit('cancel')">{{ msg.cancel }}</BaseButton>
      <BaseButton type="submit" variant="primary" :loading="saving">{{ msg.save }}</BaseButton>
    </div>
  </form>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: var(--s-3);
  padding: var(--s-3);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--bone);
}

.title {
  margin: 0;
  font-size: var(--font-size-sm);
}

.form-error {
  margin: 0;
  color: var(--danger);
  font-size: var(--font-size-sm);
}

.actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--s-2);
}
</style>
