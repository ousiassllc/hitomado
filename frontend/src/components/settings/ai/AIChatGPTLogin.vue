<script setup lang="ts">
import { computed, ref } from 'vue'
import type { ChatGPTAccount } from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { parseBindingError } from '../../../lib/api'
import { settings as msg } from '../../../lib/messages'
import { useSettingsStore } from '../../../stores/settings'
import { useUiStore } from '../../../stores/ui'
import BaseButton from '../../ui/BaseButton.vue'
import ConfirmDialog from '../../ui/ConfirmDialog.vue'
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
    const err = parseBindingError(e)
    if (err.details?.reason === 'account_signed_out') {
      // 画面が古く、ログアウト済みのアカウントを選んだ。次にすることを伝え、一覧を最新にして「再ログイン」を出す。
      error.value = msg.ai.chatgpt.selectSignedOut
      await store.load()
    } else {
      error.value = err.message
    }
    // 押したラジオが選択されたまま残らないよう、使用中のアカウントの表示へ戻す（要素は作り直さない）。
    resyncRadios()
  } finally {
    busy.value = undefined
  }
}

/** ログアウトの確認を出しているアカウント。removeTarget と同じく、閉じたあとも持ち続ける。 */
const logoutTarget = ref<ChatGPTAccount>()
const logoutOpen = ref(false)

function askLogout(a: ChatGPTAccount): void {
  logoutTarget.value = a
  logoutOpen.value = true
}

async function logout(): Promise<void> {
  const a = logoutTarget.value
  if (!a || busy.value) return
  error.value = undefined
  busy.value = 'logout'
  busyId.value = a.id
  try {
    const revoked = await store.signOutChatGPT(a.id)
    if (revoked) ui.pushToast('success', msg.ai.chatgpt.signedOut)
    else ui.pushNotice(msg.ai.chatgpt.revokeUnconfirmed)
  } catch (e) {
    error.value = parseBindingError(e).message
  } finally {
    busy.value = undefined
    busyId.value = undefined
    // 失敗の理由はこの画面の中に出るので、確認は閉じて見えるようにする。
    logoutOpen.value = false
  }
}

/** 登録の削除の確認を出しているアカウント。閉じたあとも、フェードアウトの間に見出しが空にならないよう持ち続ける。 */
const removeTarget = ref<ChatGPTAccount>()
const removeOpen = ref(false)

function askRemove(a: ChatGPTAccount): void {
  removeTarget.value = a
  removeOpen.value = true
}

async function remove(): Promise<void> {
  const a = removeTarget.value
  if (!a || busy.value) return
  error.value = undefined
  busy.value = 'remove'
  try {
    await store.removeChatGPTAccount(a.id)
    ui.pushToast('success', msg.ai.chatgpt.removed)
  } catch (e) {
    error.value = parseBindingError(e).message
  } finally {
    busy.value = undefined
    // 失敗の理由はこの画面の中に出るので、確認は閉じて見えるようにする。
    removeOpen.value = false
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
          @logout="askLogout(a)"
          @relogin="login(a.id)"
          @remove="askRemove(a)"
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

    <ConfirmDialog
      v-model:open="logoutOpen"
      :title="msg.ai.chatgpt.logoutTitle(logoutTarget ? label(logoutTarget) : '')"
      :detail="msg.ai.chatgpt.logoutDetail"
      :confirm-label="msg.ai.chatgpt.logoutConfirm"
      :busy="busy === 'logout'"
      @confirm="logout"
    />
    <ConfirmDialog
      v-model:open="removeOpen"
      :title="msg.ai.chatgpt.removeTitle(removeTarget ? label(removeTarget) : '')"
      :detail="msg.ai.chatgpt.removeDetail"
      :confirm-label="msg.ai.chatgpt.removeConfirm"
      :busy="busy === 'remove'"
      @confirm="remove"
    />
  </div>
</template>

<style scoped>
.chatgpt {
  display: flex;
  flex-direction: column;
  gap: var(--gap-field);
}

.sub {
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.note {
  margin: 0;
  color: var(--ink);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.waiting {
  margin: 0;
  color: var(--mute);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.status {
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.accounts {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
  margin: 0;
  padding: var(--pad-card-sm);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
}

.legend {
  padding: 0 var(--s-1);
  font-size: var(--font-size-sm);
  color: var(--mute);
}

.list {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.error {
  margin: 0;
  color: var(--danger);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--s-2);
}
</style>
