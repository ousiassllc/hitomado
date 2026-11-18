<script setup lang="ts">
import { computed } from 'vue'
import { type NotifyScope, notifyScopeOf, setNotifyScope } from '../../../composables/useNotifyScopeSetting'
import { parseBindingError } from '../../../lib/api'
import { settings as msg } from '../../../lib/messages'
import { useSettingsStore } from '../../../stores/settings'
import { useUiStore } from '../../../stores/ui'

// 設定「通知するメール」（メインだけ／すべて。既定はメインだけ。#395）。全アカウント共通で、アカウントごとの通知のオン・オフとは別に効く。
const store = useSettingsStore()
const ui = useUiStore()

const scope = computed(() => notifyScopeOf(store.appInfo?.notifyScope))
const options: { value: NotifyScope; label: string }[] = [
  { value: 'primary', label: msg.general.notifyScopePrimary },
  { value: 'all', label: msg.general.notifyScopeAll },
]

// 選択の表示は保存済みの値（appInfo）だけから決まるので、保存に失敗すれば appInfo が変わらず、表示も元のまま。
async function choose(next: NotifyScope): Promise<void> {
  if (next === scope.value) return
  try {
    await setNotifyScope(next)
  } catch (e) {
    ui.pushAlert(parseBindingError(e).message)
  }
}
</script>

<template>
  <div class="block" data-testid="notify-scope" :data-scope="scope">
    <strong id="notify-scope-title">{{ msg.general.notifyScope }}</strong>
    <p id="notify-scope-hint" class="body">{{ msg.general.notifyScopeHint }}</p>
    <!-- ガイドのセグメント（.seg。テーマ切替と同じ「グループ＋aria-pressed」。ラジオにはしない）。 -->
    <fieldset class="seg" aria-labelledby="notify-scope-title" aria-describedby="notify-scope-hint">
      <button
        v-for="o in options"
        :key="o.value"
        type="button"
        class="seg-btn"
        :data-value="o.value"
        :aria-pressed="scope === o.value"
        @click="choose(o.value)"
      >
        {{ o.label }}
      </button>
    </fieldset>
  </div>
</template>

<style scoped>
.block {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
  min-width: 0;
}

.body {
  margin: 0;
  color: var(--ink);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

/* ガイドの .seg：ピル型の枠（1px --border・--r-full・padding 3px・--surface 地）に、選択中だけ --accent 地・--bone 字の塗り。狭い Android でも 2 つが横に並ぶ。 */
.seg {
  display: inline-flex;
  align-self: flex-start;
  min-width: 0;
  margin: 0;
  padding: 3px;
  border: 1px solid var(--border);
  border-radius: var(--r-full);
  background: var(--surface);
}

.seg-btn {
  position: relative;
  display: inline-flex;
  align-items: center;
  border: 0;
  border-radius: var(--r-full);
  background: none;
  color: var(--mute);
  font-weight: 600;
  font-size: var(--font-size-sm);
  line-height: 1.2;
  white-space: nowrap;
  padding: 6px 14px;
  cursor: pointer;
}

/* 見た目の高さは変えず、当たり判定だけ縦に広げて 44px 以上にする（ガイドの .seg と同じ）。 */
.seg-btn::before {
  content: '';
  position: absolute;
  inset: -8px 0;
}

.seg-btn[aria-pressed='true'] {
  background: var(--accent);
  color: var(--bone);
}

.seg-btn:focus-visible {
  outline: 3px solid var(--accent);
  outline-offset: 2px;
}

@media (forced-colors: active) {
  .seg-btn[aria-pressed='true'] {
    background: Highlight;
    color: HighlightText;
    forced-color-adjust: none;
  }
}
</style>
