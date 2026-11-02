<script setup lang="ts">
import { parseBindingError } from '../../lib/api'
import { common } from '../../lib/messages'
import { SHORTCUT_KEYS } from '../../lib/shortcuts'
import { useAccountsStore } from '../../stores/accounts'
import { useComposeStore } from '../../stores/compose'
import { useInboxStore } from '../../stores/inbox'
import { useUiStore } from '../../stores/ui'
import AccountList from '../account/AccountList.vue'
import FilterBar from '../account/FilterBar.vue'
import BaseButton from '../ui/BaseButton.vue'
import BaseIconButton from '../ui/BaseIconButton.vue'
import BaseTooltip from '../ui/BaseTooltip.vue'

const ui = useUiStore()
const accounts = useAccountsStore()
const inbox = useInboxStore()
const compose = useComposeStore()

function onSelect(ids: string[]): void {
  void inbox.setFilter({ accountIds: ids, unreadOnly: inbox.filter.unreadOnly })
}

async function reconnect(id: string): Promise<void> {
  try {
    await accounts.reconnect(id)
  } catch (e) {
    const err = parseBindingError(e)
    if (err.code !== 'OAUTH_CANCELLED') ui.pushToast('error', err.message, () => void reconnect(id))
  }
}
</script>

<template>
  <aside class="sidebar" :aria-label="common.sidebar.label">
    <div class="top">
      <BaseTooltip :text="common.shortcutHint(common.sidebar.compose, SHORTCUT_KEYS.compose)">
        <BaseButton variant="primary" class="compose" @click="compose.open({ mode: 'new' })">
          {{ common.sidebar.compose }}
        </BaseButton>
      </BaseTooltip>
    </div>
    <div class="accounts">
      <AccountList @select="onSelect" @reconnect="reconnect" />
    </div>
    <FilterBar />
    <div class="bottom">
      <BaseButton variant="ghost" @click="ui.openDialog('addAccount')">＋ {{ common.sidebar.addAccount }}</BaseButton>
      <BaseIconButton :label="common.sidebar.settings" @click="ui.openSettings()">⚙</BaseIconButton>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  background: var(--color-bg-subtle);
  border-right: 1px solid var(--color-border);
}

.top {
  padding: var(--space-3);
}

.compose {
  width: 100%;
}

.accounts {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 0 var(--space-2) var(--space-2);
}

.bottom {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-2) var(--space-3);
  border-top: 1px solid var(--color-border);
}
</style>
