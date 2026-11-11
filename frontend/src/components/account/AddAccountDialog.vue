<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { Account } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { parseBindingError } from '../../lib/api'
import { common } from '../../lib/messages'
import { useAccountsStore } from '../../stores/accounts'
import { useUiStore } from '../../stores/ui'
import ColorPicker from '../settings/accounts/ColorPicker.vue'
import BaseButton from '../ui/BaseButton.vue'
import BaseDialog from '../ui/BaseDialog.vue'
import BaseSelect from '../ui/BaseSelect.vue'
import Spinner from '../ui/Spinner.vue'

type Step = 'choose' | 'waiting' | 'customize'

const emit = defineEmits<{ done: [account: Account] }>()

const ui = useUiStore()
const accounts = useAccountsStore()

const step = ref<Step>('choose')
const clientId = ref<string>()
const added = ref<Account>()
const displayName = ref('')
const color = ref('blue')
const saving = ref(false)
const error = ref('')

const t = common.addAccount
const open = computed({
  get: () => ui.dialogs.addAccount,
  set: (v: boolean) => {
    if (v) return
    if (step.value === 'waiting') void cancelWaiting()
    ui.closeDialog('addAccount')
  },
})
const options = computed(() => accounts.oauthClients.map((c) => ({ value: c.id, label: c.name })))
const title = computed(() => (step.value === 'customize' ? t.customizeTitle : t.title))

watch(open, async (isOpen) => {
  if (!isOpen) return
  step.value = 'choose'
  added.value = undefined
  error.value = ''
  try {
    await accounts.loadOAuthClients()
  } catch (e) {
    error.value = parseBindingError(e).message
  }
  clientId.value = accounts.oauthClients[0]?.id
})

function openClientSettings(): void {
  ui.closeDialog('addAccount')
  ui.openSettings()
}

async function start(): Promise<void> {
  if (!clientId.value) return
  error.value = ''
  step.value = 'waiting'
  try {
    const a = await accounts.add(clientId.value)
    added.value = a
    displayName.value = a.displayName
    color.value = a.color
    step.value = 'customize'
  } catch (e) {
    const err = parseBindingError(e)
    step.value = 'choose'
    if (err.code !== 'OAUTH_CANCELLED') error.value = err.message
  }
}

async function cancelWaiting(): Promise<void> {
  await accounts.cancelAdd()
}

async function save(): Promise<void> {
  const a = added.value
  if (!a) return
  const name = displayName.value.trim()
  const patch = {
    displayName: name && name !== a.displayName ? name : undefined,
    color: color.value !== a.color ? color.value : undefined,
  }
  let result = a
  if (patch.displayName !== undefined || patch.color !== undefined) {
    saving.value = true
    try {
      result = await accounts.update(a.id, patch)
    } catch {
      ui.pushAlert(t.updateFailed)
    } finally {
      saving.value = false
    }
  }
  ui.pushToast('success', t.added(result.displayName))
  ui.closeDialog('addAccount')
  emit('done', result)
}
</script>

<template>
  <BaseDialog
    v-model:open="open"
    :title="title"
    size="sm"
    :busy="step === 'customize' && saving"
    :body-label="step === 'waiting' ? t.waiting : undefined"
  >
    <!-- 1. OAuth クライアント選択（未登録なら設定へ誘導） -->
    <template v-if="step === 'choose'">
      <div v-if="options.length === 0" class="stack">
        <div class="group">
          <h3 class="title">{{ t.noClientTitle }}</h3>
          <p class="body">{{ t.noClientBody }}</p>
        </div>
        <BaseButton variant="primary" @click="openClientSettings">{{ t.openClientSettings }}</BaseButton>
      </div>
      <div v-else class="stack">
        <p class="muted">{{ t.chooseClient }}</p>
        <BaseSelect v-model="clientId" :options="options" :ariaLabel="t.clientLabel" />
      </div>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
    </template>

    <!-- 2. ブラウザでの認可待ち -->
    <div v-else-if="step === 'waiting'" class="stack waiting" role="status">
      <Spinner />
      <p class="waiting-text">{{ t.waiting }}</p>
      <p class="muted">{{ t.waitingHint }}</p>
    </div>

    <!-- 3. 表示名・色 -->
    <form v-else id="add-account-form" class="stack" @submit.prevent="save">
      <p class="muted">{{ added?.email }}</p>
      <label class="field">
        <span>{{ t.displayName }}</span>
        <input v-model="displayName" class="text" type="text" maxlength="40" required />
      </label>
      <ColorPicker v-model="color" />
    </form>

    <template #footer>
      <template v-if="step === 'choose'">
        <BaseButton @click="open = false">{{ common.cancel }}</BaseButton>
        <BaseButton variant="primary" :disabled="!clientId" @click="start">{{ t.start }}</BaseButton>
      </template>
      <BaseButton v-else-if="step === 'waiting'" @click="cancelWaiting">{{ t.cancelWaiting }}</BaseButton>
      <BaseButton v-else variant="primary" type="submit" form="add-account-form" :loading="saving">{{ t.done }}</BaseButton>
    </template>
  </BaseDialog>
</template>

<style scoped>
.stack {
  display: flex;
  flex-direction: column;
  gap: var(--gap-field);
}

/* 見出しとその説明文の組。組の中は --s-2、組どうしは親の gap（--gap-field）。 */
.group {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

.title {
  margin: 0;
  font-weight: 600;
  font-size: var(--font-size-base);
}

.waiting {
  align-items: center;
  text-align: center;
}

.waiting-text {
  margin: 0;
}

.muted {
  margin: 0;
  color: var(--mute);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.body {
  margin: 0;
  color: var(--ink);
  font-size: var(--font-size-base);
  line-height: var(--line-height-base);
}

.field {
  display: flex;
  flex-direction: column;
  gap: var(--gap-label);
  font-size: var(--font-size-sm);
}

.text {
  height: var(--control-h);
  padding: 0 var(--s-3);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  color: var(--ink);
  font-size: var(--font-size-sm);
}

.text:focus {
  border-color: var(--focus);
  outline: 2px solid var(--focus);
  outline-offset: -1px;
}

.text:disabled {
  opacity: 0.5;
}

.error {
  margin: var(--gap-help) 0 0;
  color: var(--danger);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}
</style>
