<script setup lang="ts">
import { ref } from 'vue'
import { parseBindingError } from '../../../lib/api'
import { settings as msg } from '../../../lib/messages'
import { useSettingsStore } from '../../../stores/settings'
import { useUiStore } from '../../../stores/ui'
import BaseButton from '../../ui/BaseButton.vue'

// Sign in with ChatGPT（OpenAI の公式フロー）。ログインはシステムブラウザの OpenAI の画面で行い、
// ここではパスワードも Cookie も扱わない。トークンは Go 側の資格情報ストアにだけあり、画面には渡らない。
const props = defineProps<{ signedIn: boolean; email: string }>()
const emit = defineEmits<{ signedIn: [] }>()

const store = useSettingsStore()
const ui = useUiStore()

const busy = ref<'login' | 'logout'>()
const error = ref<string>()

async function login(differentAccount: boolean): Promise<void> {
  if (busy.value) return
  error.value = undefined
  busy.value = 'login'
  try {
    await store.signInChatGPT(differentAccount)
    ui.pushToast('success', msg.ai.chatgpt.signedIn)
    emit('signedIn')
  } catch (e) {
    const err = parseBindingError(e)
    // 利用者が自分で中止したときは、エラーとして出さない。
    if (err.code !== 'OAUTH_CANCELLED') error.value = err.message
  } finally {
    busy.value = undefined
  }
}

async function cancel(): Promise<void> {
  try {
    await store.cancelSignInChatGPT()
  } catch (e) {
    error.value = parseBindingError(e).message
  }
}

async function logout(): Promise<void> {
  if (busy.value) return
  error.value = undefined
  busy.value = 'logout'
  try {
    const revoked = await store.signOutChatGPT()
    ui.pushToast(revoked ? 'success' : 'info', revoked ? msg.ai.chatgpt.signedOut : msg.ai.chatgpt.revokeUnconfirmed)
  } catch (e) {
    error.value = parseBindingError(e).message
  } finally {
    busy.value = undefined
  }
}
</script>

<template>
  <div class="chatgpt" data-testid="ai-chatgpt">
    <h4 class="sub">{{ msg.ai.chatgpt.heading }}</h4>
    <p class="note">{{ msg.ai.chatgpt.intro }}</p>
    <p class="note">{{ msg.ai.chatgpt.notOfficial }}</p>
    <p class="status" role="status" data-testid="ai-chatgpt-status">
      <strong>{{ props.signedIn ? msg.ai.chatgpt.statusSignedIn(props.email) : msg.ai.chatgpt.statusSignedOut }}</strong>
    </p>
    <p v-if="props.signedIn" class="plan">
      <span>{{ msg.ai.chatgpt.usingPlan }}</span>
      <BaseButton variant="ghost" @click="store.openExternal(msg.ai.chatgpt.usageUrl)">
        {{ msg.ai.chatgpt.manageUsage }}
      </BaseButton>
    </p>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <div class="actions">
      <template v-if="busy === 'login'">
        <span class="waiting" role="status">{{ msg.ai.chatgpt.loginWaiting }}</span>
        <BaseButton variant="secondary" @click="cancel">{{ msg.ai.chatgpt.loginCancel }}</BaseButton>
      </template>
      <template v-else>
        <BaseButton
          v-if="props.signedIn"
          variant="danger"
          :loading="busy === 'logout'"
          :disabled="busy !== undefined"
          @click="logout"
        >
          {{ msg.ai.chatgpt.logout }}
        </BaseButton>
        <BaseButton
          v-if="props.signedIn"
          variant="secondary"
          :disabled="busy !== undefined"
          @click="login(true)"
        >
          {{ msg.ai.chatgpt.loginDifferent }}
        </BaseButton>
        <BaseButton
          v-else
          variant="primary"
          :disabled="busy !== undefined"
          data-testid="ai-chatgpt-login"
          @click="login(false)"
        >
          {{ msg.ai.chatgpt.login }}
        </BaseButton>
      </template>
    </div>
  </div>
</template>

<style scoped>
.chatgpt {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.sub {
  margin: 0;
  font-size: var(--font-size-md);
}

.note,
.waiting {
  margin: 0;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}

.status {
  margin: 0;
  font-size: var(--font-size-sm);
}

.plan {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin: 0;
  font-size: var(--font-size-sm);
}

.error {
  margin: 0;
  color: var(--color-danger);
  font-size: var(--font-size-sm);
}

.actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--space-2);
}
</style>
