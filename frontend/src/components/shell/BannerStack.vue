<script setup lang="ts">
import { WarningTriangle } from '@iconoir/vue'
import { computed } from 'vue'
import { parseBindingError } from '../../lib/api'
import { deriveBanners, selectBanners } from '../../lib/banners'
import { useAccountsStore } from '../../stores/accounts'
import { useUiStore } from '../../stores/ui'
import BaseButton from '../ui/BaseButton.vue'

const ui = useUiStore()
const accounts = useAccountsStore()

async function reconnect(id: string): Promise<void> {
  try {
    await accounts.reconnect(id)
  } catch (e) {
    const err = parseBindingError(e)
    if (err.code !== 'OAUTH_CANCELLED') ui.pushAlert(err.message, () => void reconnect(id))
  }
}

const banners = computed(() =>
  selectBanners(
    deriveBanners({
      online: ui.online,
      accounts: accounts.accounts,
      statuses: accounts.statuses,
      statusMessages: accounts.statusMessages,
      extra: ui.banners,
      reconnect: (id) => void reconnect(id),
      openOAuthClients: () => ui.openSettings('oauth'),
    }),
  ),
)
</script>

<template>
  <div v-if="banners.length > 0" class="banners">
    <div v-for="b in banners" :key="b.key" class="banner" :class="`banner-${b.kind}`" role="status">
      <WarningTriangle class="icon icon-sm banner-icon" aria-hidden="true" />
      <span class="msg">{{ b.message }}</span>
      <BaseButton size="sm" v-if="b.action" @click="b.action.run()">{{ b.action.label }}</BaseButton>
    </div>
  </div>
</template>

<style scoped>
.banners {
  display: flex;
  flex-direction: column;
}

.banner {
  display: flex;
  align-items: center;
  gap: var(--s-3);
  padding: var(--s-2) var(--s-4);
  background: var(--warning-soft);
  color: var(--ink);
  border-bottom: 1px solid var(--warning-border);
  font-size: var(--font-size-sm);
}

.banner-icon {
  flex: none;
  color: var(--warning);
}

.msg {
  flex: 1;
  min-width: 0;
}
</style>
