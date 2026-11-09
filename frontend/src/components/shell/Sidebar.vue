<script setup lang="ts">
import { Plus, Settings } from '@iconoir/vue'
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

// ドロワー（2 ペイン・1 画面ずつ）で開いているときは、操作のあと閉じて一覧を見せる（常時表示のときは何もしない）。
function onSelect(ids: string[]): void {
  void inbox.setFilter({ accountIds: ids, unreadOnly: inbox.filter.unreadOnly })
  ui.closeMenu()
}

function composeNew(): void {
  void compose.open({ mode: 'new' })
  ui.closeMenu()
}

function openSettings(): void {
  ui.openSettings()
  ui.closeMenu()
}

function addAccount(): void {
  ui.openDialog('addAccount')
  ui.closeMenu()
}

async function reconnect(id: string): Promise<void> {
  try {
    await accounts.reconnect(id)
  } catch (e) {
    const err = parseBindingError(e)
    if (err.code !== 'OAUTH_CANCELLED') ui.pushAlert(err.message, () => void reconnect(id))
  }
}
</script>

<template>
  <aside class="sidebar" :aria-label="common.sidebar.label">
    <div class="top">
      <BaseTooltip :text="common.shortcutHint(common.sidebar.compose, SHORTCUT_KEYS.compose)">
        <BaseButton size="sm" variant="primary" class="compose" @click="composeNew">
          {{ common.sidebar.compose }}
        </BaseButton>
      </BaseTooltip>
    </div>
    <div class="accounts">
      <AccountList @select="onSelect" @reconnect="reconnect" />
    </div>
    <FilterBar />
    <div class="bottom">
      <BaseButton size="sm" variant="ghost" @click="addAccount"><Plus class="icon icon-sm" aria-hidden="true" /> {{ common.sidebar.addAccount }}</BaseButton>
      <BaseIconButton :label="common.sidebar.settings" @click="openSettings"><Settings class="icon" aria-hidden="true" /></BaseIconButton>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  background: var(--surface);
  border-right: 1px solid var(--border);
}

.top {
  padding: var(--s-3);
}

.compose {
  width: 100%;
}

.accounts {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  /* 先頭の「すべて」ボタンの外側のリング（offset 2px + 幅 2px = 4px）が上端で切れないよう、上は --s-1 の余白で収める。 */
  padding: var(--s-1) var(--s-2) var(--s-2);
}

.bottom {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--s-2) var(--s-3);
  border-top: 1px solid var(--border);
}
</style>
