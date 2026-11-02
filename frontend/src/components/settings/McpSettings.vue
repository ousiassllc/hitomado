<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { parseBindingError } from '../../lib/api'
import { settings as msg } from '../../lib/messages'
import { useSettingsStore } from '../../stores/settings'
import { useUiStore } from '../../stores/ui'
import BaseButton from '../ui/BaseButton.vue'
import BaseSwitch from '../ui/BaseSwitch.vue'
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

async function run(fn: () => Promise<void>): Promise<void> {
  if (busy.value) return
  busy.value = true
  try {
    await fn()
  } catch (e) {
    ui.pushToast('error', parseBindingError(e).message)
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

async function revoke(): Promise<void> {
  await run(async () => {
    await store.revokeMcpToken()
    shownToken.value = undefined
    ui.pushToast('success', msg.mcp.revoked)
  })
}

async function copy(text: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(text)
    ui.pushToast('success', msg.mcp.copied)
  } catch {
    ui.pushToast('error', msg.mcp.copyFailed)
  }
}
</script>

<template>
  <section class="mcp" :aria-label="msg.mcp.heading">
    <h3 class="heading">{{ msg.mcp.heading }}</h3>
    <p class="hint">{{ msg.mcp.intro }}</p>

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
      <p class="hint">{{ msg.mcp.enableHint }}</p>
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
      <p class="hint">{{ msg.mcp.allowNonLoopbackHint }}</p>
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
        <BaseButton v-if="status.hasToken" :loading="busy" @click="revoke">{{ msg.mcp.revoke }}</BaseButton>
      </div>
      <p v-if="status.hasToken" class="hint">{{ msg.mcp.reissueHint }}</p>

      <div v-if="shownToken" class="token-box" role="status">
        <strong>{{ msg.mcp.tokenShown }}</strong>
        <code class="token" data-testid="mcp-token-value">{{ shownToken }}</code>
        <BaseButton @click="copy(shownToken)">{{ msg.mcp.copy }}</BaseButton>
      </div>
      <p class="hint">{{ msg.mcp.clientHint }}</p>
    </div>
  </section>
</template>

<style scoped>
.mcp,
.block {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.heading {
  margin: 0;
  font-size: var(--font-size-lg);
}

.toggle,
.actions {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.addr {
  display: grid;
  grid-template-columns: 1fr 140px;
  gap: var(--space-3);
}

.hint {
  margin: 0;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}

.error {
  margin: 0;
  color: var(--color-danger);
  font-size: var(--font-size-sm);
}

.state {
  margin-left: auto;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}

.state.on {
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

.token-box {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-bg-subtle);
}

.token {
  overflow-wrap: anywhere;
  user-select: all;
}
</style>
