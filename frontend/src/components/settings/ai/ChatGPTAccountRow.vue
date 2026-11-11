<script setup lang="ts">
import type { ChatGPTAccount } from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { settings as msg } from '../../../lib/messages'
import BaseButton from '../../ui/BaseButton.vue'

// ChatGPT アカウント 1 件の行（使うアカウントの選択・ログアウト・再ログイン・登録の削除）。状態は持たず、操作は親へ伝える。
// busy は何かの操作の途中（ボタンを押せない）、radioDisabled はラジオを押せない状態（切り替えの途中は押せるままにしてフォーカスを保つ）。
defineProps<{ account: ChatGPTAccount; label: string; busy: boolean; radioDisabled: boolean; loggingOut: boolean }>()
defineEmits<{ select: []; logout: []; relogin: []; remove: [] }>()
</script>

<template>
  <li class="account" :data-testid="`ai-chatgpt-account-${account.id}`">
    <label v-if="account.signedIn" class="pick">
      <input
        class="radio"
        type="radio"
        name="chatgpt-account"
        :value="account.id"
        :checked="account.active"
        :aria-label="msg.ai.chatgpt.select(label)"
        :disabled="radioDisabled"
        @change="$emit('select')"
      />
      <span class="who">{{ label }}</span>
      <span v-if="account.active" class="badge badge-active" data-testid="ai-chatgpt-active">{{ msg.ai.chatgpt.accountActive }}</span>
    </label>
    <span v-else class="pick">
      <span class="who muted">{{ label }}</span>
      <span class="badge badge-signed-out">{{ msg.ai.chatgpt.accountSignedOut }}</span>
    </span>
    <span class="row-actions">
      <BaseButton size="sm"
        v-if="account.signedIn"
        variant="danger"
        :loading="loggingOut"
        :disabled="busy"
        :aria-label="msg.ai.chatgpt.logoutAccount(label)"
        data-testid="ai-chatgpt-logout"
        @click="$emit('logout')"
      >
        {{ msg.ai.chatgpt.logout }}
      </BaseButton>
      <template v-else>
        <BaseButton size="sm"
          variant="secondary"
          :disabled="busy"
          :aria-label="msg.ai.chatgpt.reloginAccount(label)"
          data-testid="ai-chatgpt-relogin"
          @click="$emit('relogin')"
        >
          {{ msg.ai.chatgpt.relogin }}
        </BaseButton>
        <BaseButton size="sm"
          variant="ghost"
          :disabled="busy"
          :aria-label="msg.ai.chatgpt.removeAccount(label)"
          data-testid="ai-chatgpt-remove"
          @click="$emit('remove')"
        >
          {{ msg.ai.chatgpt.remove }}
        </BaseButton>
      </template>
    </span>
  </li>
</template>

<style scoped>
.account {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--s-2);
}

.pick {
  display: inline-flex;
  align-items: center;
  gap: var(--s-2);
  min-width: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

/* ガイドのラジオ（O8）。ブラウザ標準にせず、外枠 --mute・18px・選択時は --accent 地に --bone の点・フォーカスは --focus の outline（3px・offset 2px）。無効は opacity .5（フォームの無効の値）。 */
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
  outline: 3px solid var(--focus);
  outline-offset: 2px;
}

.radio:disabled {
  opacity: 0.5;
  cursor: default;
}

.who {
  overflow-wrap: anywhere;
}

.muted {
  color: var(--mute);
}

.badge {
  padding: 0 var(--s-2);
  border: 1px solid var(--badge-border);
  border-radius: var(--r-full);
  background: var(--badge-soft);
  color: var(--badge-color);
  font-size: var(--font-size-xs);
  line-height: var(--line-height-xs);
}

.badge-active {
  --badge-color: var(--success);
  --badge-border: var(--success-border);
  --badge-soft: var(--success-soft);
}

.badge-signed-out {
  --badge-color: var(--warning);
  --badge-border: var(--warning-border);
  --badge-soft: var(--warning-soft);
}

.row-actions {
  display: inline-flex;
  gap: var(--s-2);
  flex-shrink: 0;
}
</style>
