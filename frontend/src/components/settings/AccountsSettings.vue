<script setup lang="ts">
import { computed, nextTick, ref, useTemplateRef } from 'vue'
import type { Account } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { parseBindingError } from '../../lib/api'
import { settings as msg } from '../../lib/messages'
import { useAccountsStore } from '../../stores/accounts'
import { useUiStore } from '../../stores/ui'
import BaseButton from '../ui/BaseButton.vue'
import BaseDialog from '../ui/BaseDialog.vue'
import AccountRow from './AccountRow.vue'

const accountsStore = useAccountsStore()
const ui = useUiStore()
const root = useTemplateRef<HTMLElement>('root')

const accounts = computed(() => accountsStore.accounts)
const busyId = ref<string>()
const removeTarget = ref<Account>()
const announcement = ref('')

function statusOf(a: Account): string {
  return accountsStore.statuses[a.id] ?? a.status
}

/** 1 アカウントに対する操作を実行し、失敗は日本語メッセージのエラートーストにする。 */
async function run(id: string, op: () => Promise<unknown>, success?: string): Promise<boolean> {
  busyId.value = id
  try {
    await op()
    if (success) ui.pushToast('success', success)
    return true
  } catch (e) {
    ui.pushToast('error', parseBindingError(e).message)
    return false
  } finally {
    busyId.value = undefined
  }
}

function rename(a: Account, displayName: string): Promise<boolean> {
  return run(a.id, () => accountsStore.update(a.id, { displayName }), msg.saved)
}

function setColor(a: Account, color: string): Promise<boolean> {
  return run(a.id, () => accountsStore.update(a.id, { color }))
}

function setNotify(a: Account, notifyEnabled: boolean): Promise<boolean> {
  return run(a.id, () => accountsStore.update(a.id, { notifyEnabled }))
}

function reconnect(a: Account): Promise<boolean> {
  return run(a.id, () => accountsStore.reconnect(a.id), msg.accounts.reconnected(a.displayName))
}

async function move(a: Account, direction: -1 | 1): Promise<void> {
  const ids = accounts.value.map((x) => x.id)
  const from = ids.indexOf(a.id)
  const to = from + direction
  if (from < 0 || to < 0 || to >= ids.length) return
  ids.splice(to, 0, ...ids.splice(from, 1))
  const ok = await run(a.id, () => accountsStore.reorder(ids))
  if (!ok) return
  announcement.value = msg.accounts.moved(a.displayName, to + 1, ids.length)
  await nextTick()
  // 並べ替え後もキーボード操作を続けられるよう、移動したアカウントのボタンにフォーカスを戻す。
  const wanted = direction === -1 ? 'up' : 'down'
  const other = wanted === 'up' ? 'down' : 'up'
  const pick = (k: string) =>
    root.value?.querySelector<HTMLButtonElement>(`[data-move="${a.id}:${k}"]`) ?? null
  const target = pick(wanted)
  ;(target && !target.disabled ? target : pick(other))?.focus()
}

async function confirmRemove(): Promise<void> {
  const a = removeTarget.value
  if (!a) return
  const ok = await run(a.id, () => accountsStore.remove(a.id), msg.accounts.removed(a.displayName))
  if (ok) removeTarget.value = undefined
}

const removeOpen = computed({
  get: () => removeTarget.value !== undefined,
  set: (v: boolean) => {
    if (!v) removeTarget.value = undefined
  },
})
</script>

<template>
  <section ref="root" class="accounts" :aria-label="msg.accounts.heading">
    <div class="bar">
      <h3 class="heading">{{ msg.accounts.heading }}</h3>
      <BaseButton variant="primary" @click="ui.openDialog('addAccount')">
        {{ msg.accounts.add }}
      </BaseButton>
    </div>

    <p v-if="accounts.length === 0" class="empty">{{ msg.accounts.empty }}</p>
    <ul v-else class="list" :aria-label="msg.accounts.listLabel">
      <AccountRow
        v-for="(a, i) in accounts"
        :key="a.id"
        :account="a"
        :index="i"
        :total="accounts.length"
        :status="statusOf(a)"
        :busy="busyId === a.id"
        @rename="rename(a, $event)"
        @color="setColor(a, $event)"
        @notify="setNotify(a, $event)"
        @move="move(a, $event)"
        @reconnect="reconnect(a)"
        @remove="removeTarget = a"
      />
    </ul>

    <p class="sr-only" role="status" aria-live="polite">{{ announcement }}</p>

    <BaseDialog
      v-model:open="removeOpen"
      size="sm"
      :title="msg.accounts.removeTitle(removeTarget?.displayName ?? '')"
      :description="msg.accounts.removeDescription(removeTarget?.email ?? '')"
    >
      <p class="detail">{{ msg.accounts.removeDetail }}</p>
      <template #footer>
        <BaseButton @click="removeOpen = false">{{ msg.cancel }}</BaseButton>
        <BaseButton variant="danger" :loading="busyId === removeTarget?.id" @click="confirmRemove">
          {{ msg.accounts.removeConfirm }}
        </BaseButton>
      </template>
    </BaseDialog>
  </section>
</template>

<style scoped>
.bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-3);
}

.heading {
  margin: 0;
  font-size: var(--font-size-lg);
}

.list {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  margin: 0;
  padding: 0;
}

.empty {
  color: var(--color-text-muted);
}

.detail {
  margin: 0;
  font-size: var(--font-size-sm);
}

.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
}
</style>
