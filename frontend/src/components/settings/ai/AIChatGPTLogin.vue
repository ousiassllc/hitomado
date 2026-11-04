<script setup lang="ts">
import { computed, ref } from 'vue'
import type { ChatGPTAccount } from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { parseBindingError } from '../../../lib/api'
import { settings as msg } from '../../../lib/messages'
import { useSettingsStore } from '../../../stores/settings'
import { useUiStore } from '../../../stores/ui'
import BaseButton from '../../ui/BaseButton.vue'
import ChatGPTAccountRow from './ChatGPTAccountRow.vue'
import ChatGPTPlanNotice from './ChatGPTPlanNotice.vue'

// Sign in with ChatGPT（OpenAI の公式フロー）。ログインはシステムブラウザの OpenAI の画面で行い、
// ここではパスワードも Cookie も扱わない。トークンは Go 側の資格情報ストアにだけあり、画面には渡らない。
// 複数の ChatGPT アカウント（クライアント登録）を保存でき、下書きに使うアカウントを選べる。
const props = defineProps<{ signedIn: boolean; email: string; accounts: ChatGPTAccount[] }>()
const emit = defineEmits<{ accountChanged: [] }>()

const store = useSettingsStore()
const ui = useUiStore()

const busy = ref<'login' | 'logout' | 'select' | 'remove'>()
const error = ref<string>()
// 操作中の行だけに進行表示を出すための、操作対象のアカウント ID。
const busyId = ref<string>()
// 切り替えに失敗したとき、押したラジオを要素を作り直さずに使用中のアカウントへ戻すための一覧の要素
// （作り直すと、キーボード操作中のフォーカスが失われる）。
const listEl = ref<HTMLElement>()
function resyncRadios(): void {
  for (const el of listEl.value?.querySelectorAll<HTMLInputElement>('input[type="radio"]') ?? []) {
    el.checked = props.accounts.some((a) => a.id === el.value && a.active)
  }
}

/** 同じメールアドレスの登録が複数あるときは、登録の ID の末尾で見分ける（ラベルは登録ごとに安定）。 */
function label(a: ChatGPTAccount): string {
  const base = a.email || msg.ai.chatgpt.noEmail
  const same = props.accounts.filter((x) => (x.email || msg.ai.chatgpt.noEmail) === base)
  return same.length > 1 ? msg.ai.chatgpt.accountDuplicate(base, a.id) : base
}

const anySignedIn = computed(() => props.accounts.some((a) => a.signedIn))
const statusText = computed(() => {
  if (props.signedIn) return msg.ai.chatgpt.statusSignedIn(props.email)
  return anySignedIn.value ? msg.ai.chatgpt.statusChoose : msg.ai.chatgpt.statusSignedOut
})

/** accountId が空なら別のアカウントを追加し、指定したときはその登録で再ログインする。 */
async function login(accountId: string): Promise<void> {
  if (busy.value) return
  error.value = undefined
  busy.value = 'login'
  try {
    await store.signInChatGPT(accountId)
    ui.pushToast('success', msg.ai.chatgpt.signedIn)
    emit('accountChanged')
  } catch (e) {
    const err = parseBindingError(e)
    // 利用者が自分で中止したときは、エラーとして出さない。
    if (err.code !== 'OAUTH_CANCELLED') error.value = err.message
  } finally {
    busy.value = undefined
  }
}

// 設定画面の流れ（同意のあと・初回の案内から）が、「Continue with ChatGPT」を押したのと同じ動作を始める。
defineExpose({ login: () => login('') })

async function cancel(): Promise<void> {
  try {
    await store.cancelSignInChatGPT()
  } catch (e) {
    error.value = parseBindingError(e).message
  }
}

async function select(a: ChatGPTAccount): Promise<void> {
  if (busy.value) {
    resyncRadios() // 別の操作の途中で押されたラジオは、選択されたまま残さない
    return
  }
  if (a.active) return
  error.value = undefined
  busy.value = 'select'
  try {
    await store.selectChatGPTAccount(a.id)
    ui.pushToast('success', msg.ai.chatgpt.switched(label(a)))
    emit('accountChanged')
  } catch (e) {
    error.value = parseBindingError(e).message
    // 押したラジオが選択されたまま残らないよう、使用中のアカウントの表示へ戻す（要素は作り直さない）。
    resyncRadios()
  } finally {
    busy.value = undefined
  }
}

async function logout(a: ChatGPTAccount): Promise<void> {
  if (busy.value) return
  error.value = undefined
  busy.value = 'logout'
  busyId.value = a.id
  try {
    const revoked = await store.signOutChatGPT(a.id)
    ui.pushToast(revoked ? 'success' : 'info', revoked ? msg.ai.chatgpt.signedOut : msg.ai.chatgpt.revokeUnconfirmed)
  } catch (e) {
    error.value = parseBindingError(e).message
  } finally {
    busy.value = undefined
    busyId.value = undefined
  }
}

async function remove(a: ChatGPTAccount): Promise<void> {
  if (busy.value) return
  error.value = undefined
  busy.value = 'remove'
  try {
    await store.removeChatGPTAccount(a.id)
    ui.pushToast('success', msg.ai.chatgpt.removed)
  } catch (e) {
    error.value = parseBindingError(e).message
  } finally {
    busy.value = undefined
  }
}
</script>

<template>
  <div class="chatgpt" data-testid="ai-chatgpt">
    <ChatGPTPlanNotice :accounts="props.accounts" />
    <h4 class="sub">{{ msg.ai.chatgpt.heading }}</h4>
    <p class="note">{{ msg.ai.chatgpt.intro }}</p>
    <p class="note">{{ msg.ai.chatgpt.notOfficial }}</p>
    <p class="status" role="status" data-testid="ai-chatgpt-status">
      <strong>{{ statusText }}</strong>
    </p>
    <p v-if="props.signedIn" class="plan">
      <span>{{ msg.ai.chatgpt.usingPlan }}</span>
      <BaseButton variant="ghost" @click="store.openExternal(msg.ai.chatgpt.usageUrl)">
        {{ msg.ai.chatgpt.manageUsage }}
      </BaseButton>
    </p>

    <fieldset v-if="props.accounts.length > 0" class="accounts" data-testid="ai-chatgpt-accounts">
      <legend class="legend">{{ msg.ai.chatgpt.accountsLabel }}</legend>
      <ul ref="listEl" class="list" :aria-label="msg.ai.chatgpt.accountsHeading">
        <ChatGPTAccountRow
          v-for="a in props.accounts"
          :key="a.id"
          :account="a"
          :label="label(a)"
          :busy="busy !== undefined"
          :radio-disabled="busy !== undefined && busy !== 'select'"
          :logging-out="busy === 'logout' && busyId === a.id"
          @select="select(a)"
          @logout="logout(a)"
          @relogin="login(a.id)"
          @remove="remove(a)"
        />
      </ul>
      <p class="note">{{ msg.ai.chatgpt.accountHint }}</p>
    </fieldset>

    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <div class="actions">
      <template v-if="busy === 'login'">
        <span class="waiting" role="status">{{ msg.ai.chatgpt.loginWaiting }}</span>
        <BaseButton variant="secondary" @click="cancel">{{ msg.ai.chatgpt.loginCancel }}</BaseButton>
      </template>
      <BaseButton
        v-else-if="anySignedIn"
        variant="secondary"
        :disabled="busy !== undefined"
        data-testid="ai-chatgpt-add"
        @click="login('')"
      >
        {{ msg.ai.chatgpt.addAccount }}
      </BaseButton>
      <BaseButton
        v-else
        variant="primary"
        :disabled="busy !== undefined"
        data-testid="ai-chatgpt-login"
        @click="login('')"
      >
        {{ msg.ai.chatgpt.login }}
      </BaseButton>
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

.accounts {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
}

.legend {
  padding: 0 var(--space-1);
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
}

.list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
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
