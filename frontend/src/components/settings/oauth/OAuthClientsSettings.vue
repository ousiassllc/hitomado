<script setup lang="ts">
import { Key } from '@iconoir/vue'
import { computed, ref } from 'vue'
import type { OAuthClient } from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { parseBindingError } from '../../../lib/api'
import { settings as msg } from '../../../lib/messages'
import { useAccountsStore } from '../../../stores/accounts'
import { useSettingsStore } from '../../../stores/settings'
import { useUiStore } from '../../../stores/ui'
import BaseButton from '../../ui/BaseButton.vue'
import ConfirmDialog from '../../ui/ConfirmDialog.vue'
import EmptyState from '../../ui/EmptyState.vue'
import OAuthClientForm from './OAuthClientForm.vue'

const accountsStore = useAccountsStore()
const settingsStore = useSettingsStore()
const ui = useUiStore()

const clients = computed(() => accountsStore.oauthClients)
/** 'new' は追加フォーム、それ以外は編集中のクライアント ID */
const editing = ref<'new' | string>()
const editingClient = computed<OAuthClient | undefined>(() =>
  clients.value.find((c) => c.id === editing.value),
)
const deletingId = ref<string>()
/** 削除の確認を出しているクライアント。閉じたあとも、フェードアウトの間に見出しが空にならないよう持ち続ける。 */
const removeTarget = ref<OAuthClient>()
const removeOpen = ref(false)

function askRemove(c: OAuthClient): void {
  removeTarget.value = c
  removeOpen.value = true
}

async function remove(): Promise<void> {
  const c = removeTarget.value
  if (!c || c.accountCount > 0) return
  deletingId.value = c.id
  try {
    await accountsStore.deleteOAuthClient(c.id)
    ui.pushToast('success', msg.oauth.removed(c.name))
    removeOpen.value = false
  } catch (e) {
    // OAUTH_CLIENT_IN_USE（他経路で使用中になった場合）を含め、Go 側のメッセージを表示する。
    ui.pushAlert(parseBindingError(e).message)
  } finally {
    deletingId.value = undefined
  }
}

function openGuide(): void {
  void settingsStore.openExternal(msg.oauth.guideUrl)
}
</script>

<template>
  <section class="oauth" :aria-label="msg.tabs.oauthClients">
    <div class="bar">
      <BaseButton variant="primary" :disabled="editing !== undefined" @click="editing = 'new'">
        {{ msg.oauth.add }}
      </BaseButton>
    </div>
    <p class="intro">{{ msg.oauth.intro }}</p>
    <p class="intro">
      <a class="link" :href="msg.oauth.guideUrl" @click.prevent="openGuide">{{ msg.oauth.guide }}</a>
    </p>

    <OAuthClientForm
      v-if="editing === 'new'"
      @saved="editing = undefined"
      @cancel="editing = undefined"
    />
    <OAuthClientForm
      v-else-if="editingClient"
      :key="editingClient.id"
      :client="editingClient"
      @saved="editing = undefined"
      @cancel="editing = undefined"
    />

    <EmptyState v-if="clients.length === 0 && editing === undefined">
      <template #icon><Key class="icon icon-lg" /></template>
      {{ msg.oauth.empty }}
    </EmptyState>
    <ul v-else class="list" :aria-label="msg.oauth.listLabel">
      <li v-for="c in clients" :key="c.id" class="item" :data-client-id="c.id">
        <div class="info">
          <strong class="name">{{ c.name }}</strong>
          <span class="meta client-id">{{ c.clientId }}</span>
          <span class="meta">
            {{ c.hasSecret ? msg.oauth.secretSaved : msg.oauth.secretMissing }}
            ／ {{ msg.oauth.accountCount(c.accountCount) }}
          </span>
          <span v-if="c.accountCount > 0" :id="`inuse-${c.id}`" class="inuse">
            {{ msg.oauth.inUse(c.accountCount) }}
          </span>
        </div>
        <div class="ops">
          <BaseButton size="sm" :disabled="editing !== undefined" @click="editing = c.id">
            {{ msg.oauth.editFor(c.name) }}
          </BaseButton>
          <BaseButton
            size="sm"
            variant="danger"
            class="remove"
            :disabled="c.accountCount > 0 || deletingId === c.id"
            :loading="deletingId === c.id"
            :aria-describedby="c.accountCount > 0 ? `inuse-${c.id}` : undefined"
            @click="askRemove(c)"
          >
            {{ msg.oauth.removeFor(c.name) }}
          </BaseButton>
        </div>
      </li>
    </ul>

    <ConfirmDialog
      v-model:open="removeOpen"
      :title="msg.oauth.removeTitle(removeTarget?.name ?? '')"
      :detail="msg.oauth.removeDetail"
      :confirm-label="msg.oauth.removeConfirm"
      :busy="deletingId !== undefined && deletingId === removeTarget?.id"
      @confirm="remove"
    />
  </section>
</template>

<style scoped>
.bar {
  display: flex;
  align-items: center;
  justify-content: flex-end;
}

.intro {
  margin: var(--s-2) 0;
  color: var(--ink);
  font-size: var(--font-size-sm);
}

.link {
  color: var(--ink);
  text-decoration: underline;
  text-decoration-color: var(--mute);
  text-underline-offset: 2px;
}

.list {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
  margin: var(--s-3) 0 0;
  padding: 0;
}

.item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--s-3);
  padding: var(--s-3);
  border: 1px solid var(--border);
  border-radius: var(--r-lg);
  list-style: none;
}

.info {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.meta {
  color: var(--mute);
  font-size: var(--font-size-xs);
  overflow-wrap: anywhere;
}

.client-id {
  font-family: var(--font-mono);
}

.inuse {
  border: 1px solid var(--warning-border);
  background: var(--warning-soft);
  color: var(--warning);
  align-self: flex-start;
  margin-top: var(--s-1);
  padding: 0 var(--s-2);
  border-radius: var(--r-full);
  font-size: var(--font-size-xs);
}

.ops {
  display: flex;
  gap: var(--s-2);
}
</style>
