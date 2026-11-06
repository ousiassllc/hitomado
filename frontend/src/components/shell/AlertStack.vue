<script setup lang="ts">
import { CheckCircle, InfoCircle, WarningCircle, Xmark } from '@iconoir/vue'
import { common } from '../../lib/messages'
import { type AlertKind, useUiStore } from '../../stores/ui'
import BaseButton from '../ui/BaseButton.vue'
import BaseIconButton from '../ui/BaseIconButton.vue'

// インラインアラート（エラー）。失敗の理由を、閉じるまで読める形で出す（アイコン＋見出し＋本文。再試行できる失敗は「再試行」）。
// 開いているダイアログがあればその中（`BaseDialog`）に、なければ画面上部（`AppShell`）に出す。
const ui = useUiStore()

const ICONS = { error: WarningCircle, info: InfoCircle, success: CheckCircle } as const
const TITLES: Record<AlertKind, string> = {
  error: common.alertTitle,
  info: common.noticeTitle,
  success: common.successTitle,
}

function retry(id: number, run: (() => void) | undefined): void {
  ui.dismissAlert(id)
  run?.()
}
</script>

<template>
  <!-- 0 件でも描画し続ける（v-if でマウントごと入れ替えると、最初の 1 件の出現と最後の 1 件の消去にフェードが付かない）。 -->
  <TransitionGroup tag="div" name="alert" appear class="alerts" data-testid="alerts">
    <div
      v-for="a in ui.alerts"
      :key="a.id"
      class="alert"
      :class="`alert-${a.kind}`"
      :role="a.kind === 'error' ? 'alert' : 'status'"
    >
      <component :is="ICONS[a.kind]" class="icon alert-icon" aria-hidden="true" />
      <div class="alert-main">
        <strong class="alert-title">{{ TITLES[a.kind] }}</strong>
        <p class="alert-body">{{ a.message }}</p>
      </div>
      <BaseButton v-if="a.retry" size="sm" @click="retry(a.id, a.retry)">{{ common.retry }}</BaseButton>
      <BaseIconButton :label="common.dismiss" @click="ui.dismissAlert(a.id)">
        <Xmark class="icon" aria-hidden="true" />
      </BaseIconButton>
    </div>
  </TransitionGroup>
</template>

<style scoped>
.alerts {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

/* アラートが無いときは場所を取らない（呼び出し側が付ける padding・border・margin も含めて消す）。 */
.alerts:empty {
  display: none;
}

.alert {
  display: flex;
  align-items: flex-start;
  gap: var(--s-3);
  padding: var(--s-3) var(--s-4);
  border: 1px solid var(--alert-border);
  border-radius: var(--r-md);
  background: var(--alert-soft);
  color: var(--ink);
}

.alert-error {
  --alert-color: var(--danger);
  --alert-border: var(--danger-border);
  --alert-soft: var(--danger-soft);
}

.alert-info {
  --alert-color: var(--info);
  --alert-border: var(--info-border);
  --alert-soft: var(--info-soft);
}

.alert-success {
  --alert-color: var(--success);
  --alert-border: var(--success-border);
  --alert-soft: var(--success-soft);
}

.alert-icon {
  margin-top: var(--s-1);
  color: var(--alert-color);
}

.alert-main {
  flex: 1;
  min-width: 0;
}

.alert-title {
  display: block;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  font-weight: 600;
}

.alert-body {
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  overflow-wrap: anywhere;
}

.alert-enter-active {
  animation: ui-fade-in var(--motion-fade-in) var(--ease-out);
}

.alert-leave-active {
  animation: ui-fade-out var(--motion-fade-out) var(--ease-in) forwards;
}
</style>
