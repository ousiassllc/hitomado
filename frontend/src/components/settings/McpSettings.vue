<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { parseBindingError } from '../../lib/api'
import { settings as msg } from '../../lib/messages'
import { useSettingsStore } from '../../stores/settings'
import { useUiStore } from '../../stores/ui'
import BaseButton from '../ui/BaseButton.vue'
import BaseDisclosure from '../ui/BaseDisclosure.vue'
import BaseSwitch from '../ui/BaseSwitch.vue'
import ConfirmDialog from '../ui/ConfirmDialog.vue'
import SettingsInput from './SettingsInput.vue'

const store = useSettingsStore()
const ui = useUiStore()

const status = computed(() => store.mcp)
const host = ref('')
const port = ref('')
const allowNonLoopback = ref(false)
const portError = ref<string>()
const busy = ref(false)
/** 発行直後の値。画面を離れる（このコンポーネントを閉じる）と捨てる。ストアには持たない。 */
const shownToken = ref<string>()

function syncDraft(): void {
  const s = status.value
  if (!s) return
  host.value = s.host
  port.value = String(s.port)
  allowNonLoopback.value = s.allowNonLoopback
}

onMounted(async () => {
  await store.loadMcp()
  syncDraft()
})
watch(() => status.value?.host, syncDraft)
onBeforeUnmount(() => {
  shownToken.value = undefined
})

function parsePort(): number | undefined {
  const n = Number(port.value)
  return Number.isInteger(n) && n >= 1024 && n <= 65535 ? n : undefined
}

/** 成功したら true。失敗は日本語メッセージのエラーアラートにして false を返す。 */
async function run(fn: () => Promise<void>): Promise<boolean> {
  if (busy.value) return false
  busy.value = true
  try {
    await fn()
    return true
  } catch (e) {
    ui.pushAlert(parseBindingError(e).message)
    return false
  } finally {
    busy.value = false
  }
}

async function apply(enabled: boolean): Promise<void> {
  // オフにするときは入力欄を検証しない（不正な値が残っていても止められる。Go 側が保存済みの値を維持する）。
  const n = enabled ? parsePort() : Number(port.value) || 0
  if (n === undefined) {
    portError.value = msg.mcp.portInvalid
    return
  }
  portError.value = undefined
  await run(async () => {
    await store.setMcpConfig({
      enabled,
      host: host.value.trim(),
      port: n,
      allowNonLoopback: allowNonLoopback.value,
    })
    syncDraft()
    ui.pushToast('success', msg.mcp.applied)
  })
}

async function issue(): Promise<void> {
  await run(async () => {
    shownToken.value = await store.issueMcpToken()
    ui.pushToast('success', msg.mcp.tokenIssuedToast)
  })
}

const revokeOpen = ref(false)

async function revoke(): Promise<void> {
  const ok = await run(async () => {
    await store.revokeMcpToken()
    shownToken.value = undefined
    ui.pushToast('success', msg.mcp.revoked)
  })
  if (ok) revokeOpen.value = false
}

async function copy(text: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(text)
    ui.pushToast('success', msg.mcp.copied)
  } catch {
    ui.pushAlert(msg.mcp.copyFailed)
  }
}
</script>

<template>
  <section class="mcp" :aria-label="msg.mcp.heading">
    <h3 class="heading">{{ msg.mcp.heading }}</h3>
    <p class="body">{{ msg.mcp.intro }}</p>

    <div v-if="status" class="block">
      <div class="toggle">
        <BaseSwitch
          :model-value="status.enabled"
          :ariaLabel="msg.mcp.enableSwitch"
          @update:model-value="apply"
        />
        <strong>{{ msg.mcp.enable }}</strong>
        <span class="state" :class="{ on: status.listening }" role="status" data-testid="mcp-state">
          {{ status.listening ? msg.mcp.statusListening : msg.mcp.statusStopped }}
        </span>
      </div>
      <p class="body">{{ msg.mcp.enableHint }}</p>
      <p v-if="status.error" class="error" role="alert">{{ msg.mcp.listenError }}{{ status.error }}</p>

      <div class="addr">
        <SettingsInput v-model="host" :label="msg.mcp.host" />
        <SettingsInput v-model="port" :label="msg.mcp.port" :error="portError" />
      </div>
      <div class="toggle">
        <BaseSwitch
          :model-value="allowNonLoopback"
          :ariaLabel="msg.mcp.allowNonLoopback"
          @update:model-value="(v: boolean) => (allowNonLoopback = v)"
        />
        <strong>{{ msg.mcp.allowNonLoopback }}</strong>
      </div>
      <p class="body">{{ msg.mcp.allowNonLoopbackHint }}</p>
      <div><BaseButton :loading="busy" @click="apply(status.enabled)">{{ msg.mcp.apply }}</BaseButton></div>

      <dl class="info">
        <dt>{{ msg.mcp.endpoint }}</dt>
        <dd class="path" data-testid="mcp-endpoint">{{ status.endpoint }}</dd>
        <dt>{{ msg.mcp.token }}</dt>
        <dd data-testid="mcp-token-state">
          {{ status.hasToken ? msg.mcp.tokenIssued : msg.mcp.tokenNone }}
        </dd>
      </dl>
      <div class="actions">
        <BaseButton variant="primary" :loading="busy" @click="issue">
          {{ status.hasToken ? msg.mcp.reissue : msg.mcp.issue }}
        </BaseButton>
        <BaseButton v-if="status.hasToken" :loading="busy" @click="revokeOpen = true">{{ msg.mcp.revoke }}</BaseButton>
      </div>
      <p v-if="status.hasToken" class="body">{{ msg.mcp.reissueHint }}</p>

      <div v-if="shownToken" class="token-box" role="status">
        <strong>{{ msg.mcp.tokenShown }}</strong>
        <code class="token" data-testid="mcp-token-value">{{ shownToken }}</code>
        <BaseButton @click="copy(shownToken)">{{ msg.mcp.copy }}</BaseButton>
      </div>
      <BaseDisclosure :summary="msg.mcp.connectHeading">
        <p class="hint">{{ msg.mcp.clientHint }}</p>
      </BaseDisclosure>
    </div>

    <ConfirmDialog
      v-model:open="revokeOpen"
      :title="msg.mcp.revokeTitle"
      :detail="msg.mcp.revokeDetail"
      :confirm-label="msg.mcp.revokeConfirm"
      :busy="busy"
      @confirm="revoke"
    />
  </section>
</template>

<style scoped>
.mcp,
.block {
  display: flex;
  flex-direction: column;
  gap: var(--s-3);
}

.heading {
  margin: 0;
  font-size: var(--font-size-base);
}

.toggle,
.actions {
  display: flex;
  align-items: center;
  gap: var(--s-2);
}

.addr {
  display: grid;
  grid-template-columns: 1fr 140px;
  gap: var(--s-3);
}

.hint {
  margin: 0;
  color: var(--mute);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.body {
  margin: 0;
  color: var(--ink);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.error {
  margin: 0;
  color: var(--danger);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.state {
  margin-left: auto;
  padding: 0 var(--s-2);
  border: 1px solid var(--border);
  border-radius: var(--r-full);
  color: var(--mute);
  font-size: var(--font-size-xs);
}

.state.on {
  border-color: var(--success-border);
  background: var(--success-soft);
  color: var(--success);
}

.info {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: var(--s-1) var(--s-3);
  margin: 0;
}

.info dt {
  color: var(--mute);
}

.info dd {
  margin: 0;
}

.path {
  font-family: var(--font-mono);
  overflow-wrap: anywhere;
}

.token-box {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
  padding: var(--s-3);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--bone);
}

.token {
  font-family: var(--font-mono);
  overflow-wrap: anywhere;
  user-select: all;
}
</style>
