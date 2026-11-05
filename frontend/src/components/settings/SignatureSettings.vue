<script setup lang="ts">
import { computed, ref } from 'vue'
import { parseBindingError } from '../../lib/api'
import { settings as msg } from '../../lib/messages'
import { useAccountsStore } from '../../stores/accounts'
import { useUiStore } from '../../stores/ui'
import RichTextEditor from '../compose/RichTextEditor.vue'
import BaseButton from '../ui/BaseButton.vue'
import BaseDialog from '../ui/BaseDialog.vue'
import BaseSelect from '../ui/BaseSelect.vue'
import { normalizeSignatureHtml } from './signatureHtml'

const accountsStore = useAccountsStore()
const ui = useUiStore()

const accounts = computed(() => accountsStore.accounts)
const selectedId = ref<string>()
const current = computed(
  () => accounts.value.find((a) => a.id === selectedId.value) ?? accounts.value[0],
)
/** アカウントを切り替えても未保存の編集を失わないよう、アカウントごとに下書きを保持する。 */
const drafts = ref<Record<string, string>>({})
const saving = ref(false)

const html = computed(() => {
  const a = current.value
  if (!a) return ''
  return drafts.value[a.id] ?? a.signatureHtml
})
const dirty = computed(() => {
  const a = current.value
  return (
    a !== undefined &&
    normalizeSignatureHtml(html.value) !== normalizeSignatureHtml(a.signatureHtml)
  )
})
const options = computed(() =>
  accounts.value.map((a) => ({ value: a.id, label: `${a.displayName} <${a.email}>` })),
)

function onEdit(value: string): void {
  const a = current.value
  if (a) drafts.value = { ...drafts.value, [a.id]: value }
}

function revert(): void {
  const a = current.value
  if (!a) return
  const { [a.id]: _discarded, ...rest } = drafts.value
  drafts.value = rest
  editorKey.value++
}

// 元に戻したときにエディタを作り直して内容を反映させる。
const editorKey = ref(0)

const fetching = ref(false)
/** 置き換えの確認待ちの Gmail の署名（エディタに入力済みのときだけ使う）。 */
const pendingReplace = ref<{ accountId: string; html: string; sanitized: boolean }>()

function draftOf(accountId: string): string {
  return drafts.value[accountId] ?? accounts.value.find((a) => a.id === accountId)?.signatureHtml ?? ''
}

/** 押したときだけ Gmail の署名を取得し、エディタに入れる（保存はしない）。 */
async function fetchFromGmail(): Promise<void> {
  const a = current.value
  if (!a || fetching.value) return
  fetching.value = true
  try {
    const sig = await accountsStore.fetchGmailSignature(a.id)
    if (!sig.found) {
      ui.pushToast('info', msg.signature.fetchEmpty)
      return
    }
    if (normalizeSignatureHtml(draftOf(a.id)) === '') {
      applyGmail(a.id, a.displayName, sig.signatureHtml, sig.sanitized)
    } else {
      pendingReplace.value = { accountId: a.id, html: sig.signatureHtml, sanitized: sig.sanitized }
    }
  } catch (e) {
    ui.pushAlert(parseBindingError(e).message)
  } finally {
    fetching.value = false
  }
}

function applyGmail(accountId: string, name: string, signatureHtml: string, sanitized: boolean): void {
  drafts.value = { ...drafts.value, [accountId]: signatureHtml }
  editorKey.value++
  // 「保存すると反映されます」まで読ませたいので、1 行のトーストではなくインラインアラートで出す。
  ui.pushNotice(msg.signature.fetchedGmail(name), 'success')
  if (sanitized) ui.pushNotice(msg.signature.fetchSanitized)
}

function confirmReplace(): void {
  const p = pendingReplace.value
  pendingReplace.value = undefined
  if (!p) return
  const name = accounts.value.find((a) => a.id === p.accountId)?.displayName ?? ''
  applyGmail(p.accountId, name, p.html, p.sanitized)
}

function cancelReplace(): void {
  pendingReplace.value = undefined
}

async function save(): Promise<void> {
  const a = current.value
  if (!a || saving.value) return
  saving.value = true
  try {
    await accountsStore.setSignature(a.id, normalizeSignatureHtml(html.value))
    const { [a.id]: _saved, ...rest } = drafts.value
    drafts.value = rest
    editorKey.value++
    ui.pushToast('success', msg.signature.saved(a.displayName))
  } catch (e) {
    ui.pushAlert(parseBindingError(e).message)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <section class="sig" :aria-label="msg.tabs.signature">
    <p class="intro">{{ msg.signature.intro }}</p>

    <p v-if="!current" class="empty">{{ msg.signature.empty }}</p>
    <template v-else>
      <div class="select">
        <BaseSelect
          :model-value="current.id"
          :options="options"
          :ariaLabel="msg.signature.account"
          @update:model-value="selectedId = $event"
        />
      </div>
      <RichTextEditor
        :key="`${current.id}:${editorKey}`"
        :html="html"
        :ariaLabel="msg.signature.editorLabel(current.displayName)"
        @update:html="onEdit"
      />
      <div class="actions">
        <span v-if="dirty" class="dirty" role="status">{{ msg.signature.dirty }}</span>
        <span class="spacer" />
        <BaseButton
          :loading="fetching"
          :disabled="saving"
          :title="msg.signature.fetchHint"
          @click="fetchFromGmail"
        >
          {{ msg.signature.fetchGmail }}
        </BaseButton>
        <BaseButton :disabled="!dirty || saving" @click="revert">{{ msg.signature.revert }}</BaseButton>
        <BaseButton variant="primary" :disabled="!dirty" :loading="saving" @click="save">
          {{ msg.save }}
        </BaseButton>
      </div>
    </template>

    <BaseDialog
      :open="pendingReplace !== undefined"
      :title="msg.signature.replaceTitle"
      size="sm"
      @update:open="(v) => !v && cancelReplace()"
    >
      <p class="text">{{ msg.signature.replaceBody }}</p>
      <template #footer>
        <BaseButton variant="secondary" @click="cancelReplace">
          {{ msg.signature.replaceCancel }}
        </BaseButton>
        <BaseButton variant="primary" @click="confirmReplace">
          {{ msg.signature.replaceConfirm }}
        </BaseButton>
      </template>
    </BaseDialog>
  </section>
</template>

<style scoped>
.intro {
  margin: var(--s-2) 0 var(--s-3);
  color: var(--ink);
  font-size: var(--font-size-sm);
}

.empty {
  color: var(--mute);
}

.select {
  margin-bottom: var(--s-3);
  max-width: 420px;
}

.actions {
  display: flex;
  align-items: center;
  gap: var(--s-2);
  margin-top: var(--s-3);
}

.spacer {
  flex: 1;
}

.text {
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.dirty {
  border: 1px solid var(--warning);
  background: var(--warning-soft);
  color: var(--warning);
  padding: 0 var(--s-2);
  border-radius: var(--r-full);
  font-size: var(--font-size-xs);
}
</style>
