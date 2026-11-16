<script setup lang="ts">
import { Settings } from '@iconoir/vue'
import { parseBindingError } from '../../lib/api'
import { common } from '../../lib/messages'
import { vOverlayScroll } from '../../lib/overlayScroll'
import { SHORTCUT_KEYS } from '../../lib/shortcuts'
import { useAccountsStore } from '../../stores/accounts'
import { useComposeStore } from '../../stores/compose'
import { useUiStore } from '../../stores/ui'
import AccountList from '../account/AccountList.vue'
import BaseButton from '../ui/BaseButton.vue'
import BaseTooltip from '../ui/BaseTooltip.vue'
import SidebarAgent from './SidebarAgent.vue'
import SidebarPlaces from './SidebarPlaces.vue'

// PC（3 ペイン）のサイドバー。上から: 新規作成 → 見る場所（受信箱・下書き・送信済み・アーカイブ）→ アカウントの絞り込み
// （全アカウント・各アカウント・アカウントを追加）→ 一番下にエージェントの入口と設定。スマホはこれを使わず、下 4 タブ（BottomTabs）。
const ui = useUiStore()
const accounts = useAccountsStore()
const compose = useComposeStore()

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
        <BaseButton size="sm" variant="primary" class="compose" @click="compose.open({ mode: 'new' })">
          {{ common.sidebar.compose }}
        </BaseButton>
      </BaseTooltip>
    </div>
    <SidebarPlaces />
    <div v-overlay-scroll class="scroll">
      <AccountList @reconnect="reconnect" @add="ui.openDialog('addAccount')" />
    </div>
    <div class="bottom">
      <SidebarAgent />
      <button type="button" class="settings" @click="ui.openSettings()">
        <Settings class="icon icon-sm" aria-hidden="true" />{{ common.sidebar.settings }}
      </button>
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

.scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  /* 先頭の見出しや行の外側のリング（offset 2px + 幅 2px = 4px）が左右・上端で切れないよう、余白で収める。 */
  padding: var(--s-1) var(--s-2) var(--s-2);
}

.bottom {
  display: flex;
  flex-direction: column;
  gap: var(--gap-menu);
  padding: var(--s-2);
  border-top: 1px solid var(--border);
}

.settings {
  display: flex;
  align-items: center;
  gap: var(--s-3);
  width: 100%;
  min-height: var(--menu-item-h);
  padding: 0 var(--menu-pad-x);
  border: none;
  border-radius: var(--r-sm);
  background: transparent;
  color: var(--ink);
  font: inherit;
  font-size: var(--font-size-sm);
  text-align: left;
  cursor: pointer;
}

.settings:hover {
  background: var(--accent-soft);
}
</style>
