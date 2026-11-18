<script setup lang="ts">
import { computed, ref } from 'vue'
import { type NotifyScope, notifyScopeOf, setNotifyScope } from '../../../composables/useNotifyScopeSetting'
import { parseBindingError } from '../../../lib/api'
import { settings as msg } from '../../../lib/messages'
import { useSettingsStore } from '../../../stores/settings'
import { useUiStore } from '../../../stores/ui'

// 設定「通知するメール」（メインだけ／すべて。既定はメインだけ。#395）。全アカウント共通で、アカウントごとの通知のオン・オフとは別に効く。
const store = useSettingsStore()
const ui = useUiStore()

const scope = computed(() => notifyScopeOf(store.appInfo?.notifyScope))
const listEl = ref<HTMLElement>()

/** 押したラジオが選択されたまま残らないよう、保存に失敗したら保存済みの値の表示へ戻す（要素は作り直さない）。 */
function resyncRadios(): void {
  for (const el of listEl.value?.querySelectorAll<HTMLInputElement>('input[type="radio"]') ?? []) {
    el.checked = el.value === scope.value
  }
}

async function choose(next: NotifyScope): Promise<void> {
  try {
    await setNotifyScope(next)
  } catch (e) {
    ui.pushAlert(parseBindingError(e).message)
    resyncRadios()
  }
}
</script>

<template>
  <fieldset ref="listEl" class="field" data-testid="notify-scope" :data-scope="scope" aria-describedby="notify-scope-hint">
    <legend class="title">{{ msg.general.notifyScope }}</legend>
    <label class="pick">
      <input class="radio" type="radio" name="notify-scope" value="primary" :checked="scope === 'primary'" @change="choose('primary')" />
      <span>{{ msg.general.notifyScopePrimary }}</span>
    </label>
    <label class="pick">
      <input class="radio" type="radio" name="notify-scope" value="all" :checked="scope === 'all'" @change="choose('all')" />
      <span>{{ msg.general.notifyScopeAll }}</span>
    </label>
    <p id="notify-scope-hint" class="hint">{{ msg.general.notifyScopeHint }}</p>
  </fieldset>
</template>

<style scoped>
.field {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
  margin: 0;
  padding: 0;
  border: 0;
  min-width: 0;
}

.title {
  padding: 0;
  margin-bottom: var(--s-2);
  font-weight: 600;
  font-size: var(--font-size-base);
}

.pick {
  display: flex;
  align-items: center;
  gap: var(--s-2);
  cursor: pointer;
}

/* ガイドのラジオ（O8）。ブラウザ標準にせず、外枠 --mute・18px・選択時は --accent 地に --bone の点・フォーカスは --accent の outline（3px・offset 2px）。 */
.radio {
  appearance: none;
  flex: none;
  display: grid;
  place-content: center;
  width: 18px;
  height: 18px;
  margin: 0;
  border: 1px solid var(--mute);
  border-radius: var(--r-full);
  background: var(--surface);
  cursor: pointer;
}

.radio:checked {
  border-color: var(--accent);
  background: var(--accent);
}

.radio:checked::after {
  content: '';
  width: 6px;
  height: 6px;
  border-radius: var(--r-full);
  background: var(--bone);
}

.radio:focus-visible {
  outline: 3px solid var(--accent);
  outline-offset: 2px;
}

.hint {
  margin: 0;
  color: var(--mute);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}
</style>
