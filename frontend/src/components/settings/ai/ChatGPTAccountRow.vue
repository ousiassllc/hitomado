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
        type="radio"
        name="chatgpt-account"
        :value="account.id"
        :checked="account.active"
        :aria-label="msg.ai.chatgpt.select(label)"
        :disabled="radioDisabled"
        @change="$emit('select')"
      />
      <span class="who">{{ label }}</span>
      <span v-if="account.active" class="badge" data-testid="ai-chatgpt-active">{{ msg.ai.chatgpt.accountActive }}</span>
    </label>
    <span v-else class="pick">
      <span class="who muted">{{ label }}</span>
      <span class="badge">{{ msg.ai.chatgpt.accountSignedOut }}</span>
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
}

.who {
  overflow-wrap: anywhere;
}

.muted {
  color: var(--mute);
}

.badge {
  padding: 0 var(--s-2);
  border: 1px solid var(--border);
  border-radius: var(--r-sm);
  font-size: var(--font-size-xs);
}

.row-actions {
  display: inline-flex;
  gap: var(--s-2);
  flex-shrink: 0;
}
</style>
