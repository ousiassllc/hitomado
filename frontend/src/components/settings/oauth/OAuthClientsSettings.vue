<script setup lang="ts">
import { computed, ref } from 'vue'
import type { OAuthClient } from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { parseBindingError } from '../../../lib/api'
import { settings as msg } from '../../../lib/messages'
import { useAccountsStore } from '../../../stores/accounts'
import { useSettingsStore } from '../../../stores/settings'
import { useUiStore } from '../../../stores/ui'
import BaseButton from '../../ui/BaseButton.vue'
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

async function remove(c: OAuthClient): Promise<void> {
  if (c.accountCount > 0) return
  deletingId.value = c.id
  try {
    await accountsStore.deleteOAuthClient(c.id)
    ui.pushToast('success', msg.oauth.removed(c.name))
  } catch (e) {
    // OAUTH_CLIENT_IN_USE（他経路で使用中になった場合）を含め、Go 側のメッセージを表示する。
    ui.pushToast('error', parseBindingError(e).message)
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

    <p v-if="clients.length === 0 && editing === undefined" class="empty">{{ msg.oauth.empty }}</p>
    <ul v-else class="list" :aria-label="msg.oauth.listLabel">
      <li v-for="c in clients" :key="c.id" class="item" :data-client-id="c.id">
        <div class="info">
          <strong class="name">{{ c.name }}</strong>
          <span class="meta">{{ c.clientId }}</span>
          <span class="meta">
            {{ c.hasSecret ? msg.oauth.secretSaved : msg.oauth.secretMissing }}
            ／ {{ msg.oauth.accountCount(c.accountCount) }}
          </span>
          <span v-if="c.accountCount > 0" :id="`inuse-${c.id}`" class="inuse">
            {{ msg.oauth.inUse(c.accountCount) }}
          </span>
        </div>
        <div class="ops">
          <BaseButton :disabled="editing !== undefined" @click="editing = c.id">
            {{ msg.oauth.editFor(c.name) }}
          </BaseButton>
          <BaseButton
            variant="danger"
            class="remove"
            :disabled="c.accountCount > 0 || deletingId === c.id"
            :loading="deletingId === c.id"
            :aria-describedby="c.accountCount > 0 ? `inuse-${c.id}` : undefined"
            @click="remove(c)"
          >
            {{ msg.oauth.removeFor(c.name) }}
          </BaseButton>
        </div>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.bar {
  display: flex;
  align-items: center;
  justify-content: flex-end;
}

.intro {
  margin: var(--space-2) 0;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}

.link {
  color: var(--color-accent);
}

.empty {
  color: var(--color-text-muted);
}

.list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: var(--space-3) 0 0;
  padding: 0;
}

.item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding: var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  list-style: none;
}

.info {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.meta {
  color: var(--color-text-muted);
  font-size: var(--font-size-xs);
  overflow-wrap: anywhere;
}

.inuse {
  color: var(--color-warning-text);
  background: var(--color-warning-bg);
  align-self: flex-start;
  margin-top: var(--space-1);
  padding: 0 var(--space-2);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-xs);
}

.ops {
  display: flex;
  gap: var(--space-2);
}
</style>
