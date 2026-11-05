<script setup lang="ts">
import { ref } from 'vue'
import { parseBindingError } from '../../lib/api'
import { settings as msg } from '../../lib/messages'
import { useSettingsStore } from '../../stores/settings'
import { useUiStore } from '../../stores/ui'
import BaseButton from '../ui/BaseButton.vue'
import BaseDisclosure from '../ui/BaseDisclosure.vue'

const store = useSettingsStore()
const ui = useUiStore()
const exporting = ref(false)

async function openFolder(): Promise<void> {
  try {
    await store.openLogFolder()
  } catch (e) {
    ui.pushAlert(parseBindingError(e).message)
  }
}

async function exportLogs(): Promise<void> {
  if (exporting.value) return
  exporting.value = true
  try {
    const res = await store.exportLogs()
    if (res.saved && res.path) ui.pushNotice(msg.logs.exported(res.path), 'success')
  } catch (e) {
    ui.pushAlert(parseBindingError(e).message)
  } finally {
    exporting.value = false
  }
}
</script>

<template>
  <section class="logs" :aria-label="msg.tabs.logs">
    <p class="body">{{ msg.logs.redactHint }}</p>
    <BaseDisclosure :summary="msg.logs.storageHeading">
      <p class="body">{{ msg.logs.intro }}</p>
    </BaseDisclosure>
    <div class="actions">
      <BaseButton @click="openFolder">{{ msg.logs.openFolder }}</BaseButton>
      <BaseButton :loading="exporting" @click="exportLogs">
        {{ exporting ? msg.logs.exporting : msg.logs.export }}
      </BaseButton>
    </div>
  </section>
</template>

<style scoped>
.logs {
  display: flex;
  flex-direction: column;
  gap: var(--s-3);
}

.body {
  margin: 0;
  color: var(--ink);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.actions {
  display: flex;
  gap: var(--s-2);
}
</style>
