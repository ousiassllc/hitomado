<script setup lang="ts">
import { computed, ref } from 'vue'
import { parseBindingError } from '../../lib/api'
import { settings as msg } from '../../lib/messages'
import { usePlatformStore } from '../../stores/platform'
import { useSettingsStore } from '../../stores/settings'
import { useUiStore } from '../../stores/ui'
import BaseButton from '../ui/BaseButton.vue'
import BaseSwitch from '../ui/BaseSwitch.vue'

const store = useSettingsStore()
const ui = useUiStore()
const platform = usePlatformStore()

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
  <section class="general" :aria-label="msg.general.heading">
    <h3 class="heading">{{ msg.general.heading }}</h3>

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

.heading {
  margin: 0;
  font-size: var(--font-size-lg);
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
