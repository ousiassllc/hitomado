<script setup lang="ts">
import { ref } from 'vue'
import { parseBindingError } from '../../../../lib/api'
import { settings as msg } from '../../../../lib/messages'
import { useSettingsStore } from '../../../../stores/settings'
import { useUiStore } from '../../../../stores/ui'
import ConfirmDialog from '../../../ui/ConfirmDialog.vue'
import AIProviderRow from './AIProviderRow.vue'
import { AI_PROVIDERS, type AIProviderId } from './aiProviders'

// 「AI のつなぎ先」タブ（#376）。プロバイダごとに 1 行で、つなぐ操作（ログイン・API キー・同意）だけを置く。
// つなぐ・設定は、そのプロバイダのページ（親が開く）。接続を外すのは、確認のうえこの一覧でその場で行う。
const emit = defineEmits<{ connect: [id: AIProviderId]; open: [id: AIProviderId] }>()
const store = useSettingsStore()
const ui = useUiStore()
const t = msg.ai.connect

const target = ref<AIProviderId>()
const confirmOpen = ref(false)
const busy = ref(false)
const error = ref<string>()

function askDisconnect(id: AIProviderId): void {
  target.value = id
  error.value = undefined
  confirmOpen.value = true
}

/** ChatGPT はログイン中のアカウントすべてをログアウトし、ほかは API キーを削除する。選んでいた AI は変えない（機能の行が「つなぎ直しが必要」を出す）。 */
async function disconnect(): Promise<void> {
  const id = target.value
  if (id === undefined || busy.value) return
  busy.value = true
  try {
    if (id === 'chatgpt') {
      let unconfirmed = false
      for (const a of (store.aiConfig?.chatgptAccounts ?? []).filter((x) => x.signedIn)) {
        if (!(await store.signOutChatGPT(a.id))) unconfirmed = true
      }
      if (unconfirmed) ui.pushNotice(msg.ai.chatgpt.revokeUnconfirmed)
    } else {
      await store.clearAIApiKey(id)
    }
    ui.pushToast('success', t.disconnectedToast(msg.ai.providerList.names[id] ?? id))
  } catch (e) {
    error.value = parseBindingError(e).message
  } finally {
    busy.value = false
    confirmOpen.value = false
  }
}
</script>

<template>
  <section class="connect" :aria-label="t.heading" data-testid="ai-connect-tab">
    <div class="group">
      <h4 class="sh">{{ t.heading }}</h4>
      <p class="hint">{{ t.hint }}</p>
    </div>
    <ul class="box" :aria-label="t.listLabel" data-testid="ai-provider-list">
      <AIProviderRow
        v-for="p in AI_PROVIDERS"
        :key="p.id"
        :id="p.id"
        :busy="busy"
        @connect="emit('connect', p.id)"
        @open="emit('open', p.id)"
        @disconnect="askDisconnect(p.id)"
      />
    </ul>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <ConfirmDialog
      :open="confirmOpen"
      :title="t.disconnectTitle(msg.ai.providerList.names[target ?? 'chatgpt'] ?? '')"
      :detail="target === 'chatgpt' ? t.disconnectDetailChatGPT : t.disconnectDetail"
      :confirm-label="t.disconnect"
      :busy="busy"
      @update:open="(v) => (confirmOpen = v)"
      @confirm="disconnect"
    />
  </section>
</template>

<style scoped>
.connect {
  display: flex;
  flex-direction: column;
  gap: var(--s-3);
}

.group {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

.sh {
  margin: 0;
  font-weight: 600;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.hint {
  margin: 0;
  color: var(--mute);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.box {
  margin: 0;
  padding: 0;
  list-style: none;
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
}

.box > :deep(.prow + .prow) {
  border-top: 1px solid var(--border);
}

.error {
  margin: 0;
  color: var(--danger);
  font-size: var(--font-size-sm);
}

/* 390px 幅: 行はそれぞれ枠のあるカードにし、一覧の外枠は外す。 */
@media (max-width: 640px) {
  .box {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    border: none;
    background: transparent;
  }

  .box > :deep(.prow) {
    border: 1px solid var(--border);
    border-radius: var(--r-md);
    background: var(--surface);
  }

  .box > :deep(.prow + .prow) {
    border-top: 1px solid var(--border);
  }
}
</style>
