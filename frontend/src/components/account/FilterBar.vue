<script setup lang="ts">
import { computed } from 'vue'
import { common } from '../../lib/messages'
import { useInboxStore } from '../../stores/inbox'
import BaseButton from '../ui/BaseButton.vue'
import BaseSwitch from '../ui/BaseSwitch.vue'

const inbox = useInboxStore()
const emit = defineEmits<{ change: [] }>()

const active = computed(() => inbox.filter.unreadOnly || inbox.filter.accountIds.length > 0)

async function setUnreadOnly(v: boolean): Promise<void> {
  await inbox.setFilter({ accountIds: inbox.filter.accountIds, unreadOnly: v })
  emit('change')
}

async function clear(): Promise<void> {
  await inbox.setFilter({ accountIds: [], unreadOnly: false })
  emit('change')
}
</script>

<template>
  <div class="filter">
    <div class="row">
      <span>{{ common.filter.unreadOnly }}</span>
      <BaseSwitch :model-value="inbox.filter.unreadOnly" :ariaLabel="common.filter.unreadOnly" @update:model-value="setUnreadOnly" />
    </div>
    <BaseButton v-if="active" variant="ghost" @click="clear">{{ common.filter.clear }}</BaseButton>
  </div>
</template>

<style scoped>
.filter {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3);
  border-top: 1px solid var(--color-border);
}

.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: var(--font-size-sm);
}
</style>
