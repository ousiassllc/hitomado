<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { parseBindingError } from '../../lib/api'
import { settings as msg } from '../../lib/messages'
import { useNotificationsStore } from '../../stores/notifications'
import { usePlatformStore } from '../../stores/platform'
import { useSettingsStore } from '../../stores/settings'
import { useUiStore } from '../../stores/ui'
import BaseButton from '../ui/BaseButton.vue'
import BaseSwitch from '../ui/BaseSwitch.vue'

const store = useSettingsStore()
const ui = useUiStore()
const platform = usePlatformStore()
const notifications = useNotificationsStore()

// 通知の許可は OS の設定画面で変えられるため、設定を開いたとき・アプリへ戻ったときに読み直す。
function refreshPermission(): void {
  void notifications.refresh()
}
onMounted(() => {
  refreshPermission()
  window.addEventListener('focus', refreshPermission)
})
onBeforeUnmount(() => window.removeEventListener('focus', refreshPermission))

const info = computed(() => store.appInfo)
const update = computed(() => store.update)
const checking = ref(false)
/** 手動確認の結果（最新／更新あり）。未確認なら空。 */
const checked = ref(false)

async function toggleUpdateCheck(enabled: boolean): Promise<void> {
  try {
    await store.setUpdateCheckEnabled(enabled)
  } catch (e) {
    ui.pushToast('error', parseBindingError(e).message)
  }
}

async function checkNow(): Promise<void> {
  if (checking.value) return
  checking.value = true
  try {
    await store.checkUpdate()
    checked.value = true
  } catch (e) {
    ui.pushToast('error', parseBindingError(e).message)
  } finally {
    checking.value = false
  }
}

function openUpdate(): void {
  const url = update.value?.url
  if (url) void store.openExternal(url)
}
</script>

<template>
  <section class="general" :aria-label="msg.tabs.general">
    <!-- アプリ内の更新確認を持たない OS（Android。配布は Google Play）では出さない。 -->
    <div v-if="platform.supportsAutoUpdate" class="block">
      <div class="toggle">
        <BaseSwitch
          :model-value="info?.updateCheckEnabled ?? false"
          :ariaLabel="msg.general.updateCheckSwitch"
          @update:model-value="toggleUpdateCheck"
        />
        <strong>{{ msg.general.updateCheck }}</strong>
      </div>
      <p class="hint">{{ msg.general.updateCheckHint }}</p>
      <div class="check">
        <BaseButton :disabled="!info?.updateCheckEnabled" :loading="checking" @click="checkNow">
          {{ checking ? msg.general.checking : msg.general.checkNow }}
        </BaseButton>
        <p v-if="update?.available && update.version" class="update" role="status">
          {{ msg.general.updateAvailable(update.version) }}
          <a v-if="update.url" class="link" :href="update.url" @click.prevent="openUpdate">
            {{ msg.general.updateLink }}
          </a>
        </p>
        <p v-else-if="checked" class="hint" role="status">{{ msg.general.upToDate }}</p>
      </div>
    </div>

    <!-- 通知の許可を管理する OS（Android）だけ。許可されていないと、閉じているあいだの新着をお知らせできない。 -->
    <div
      v-if="notifications.permission !== 'unsupported'"
      class="block"
      data-testid="notification-permission"
      :data-state="notifications.permission"
    >
      <strong>{{ msg.notifications.heading }}</strong>
      <p v-if="notifications.permission === 'granted'" class="hint" role="status">
        {{ msg.notifications.granted }}
      </p>
      <template v-else>
        <p class="hint" role="status">{{ msg.notifications.denied }}</p>
        <p class="hint">{{ msg.notifications.deniedHint }}</p>
        <div class="check">
          <BaseButton @click="notifications.request()">{{ msg.notifications.request }}</BaseButton>
          <BaseButton @click="notifications.openSettings()">
            {{ msg.notifications.openSettings }}
          </BaseButton>
        </div>
      </template>
    </div>

    <dl class="info">
      <dt>{{ msg.general.version }}</dt>
      <dd data-testid="app-version">{{ info?.version ?? msg.general.unknown }}</dd>
      <dt>{{ msg.general.dataDir }}</dt>
      <dd class="path" data-testid="app-datadir">{{ info?.dataDir ?? msg.general.unknown }}</dd>
    </dl>
    <p class="hint">{{ msg.general.dataDirHint }}</p>
  </section>
</template>

<style scoped>
.general {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.block {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.toggle,
.check {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.hint {
  margin: 0;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}

.update {
  margin: 0;
  font-size: var(--font-size-sm);
}

.link {
  color: var(--color-accent);
}

.info {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: var(--space-1) var(--space-3);
  margin: 0;
}

.info dt {
  color: var(--color-text-muted);
}

.info dd {
  margin: 0;
}

.path {
  overflow-wrap: anywhere;
}
</style>
