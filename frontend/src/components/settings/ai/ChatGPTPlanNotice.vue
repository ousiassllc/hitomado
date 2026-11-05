<script setup lang="ts">
import { computed, ref } from 'vue'
import type { ChatGPTAccount } from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { settings as msg } from '../../../lib/messages'
import { useSettingsStore } from '../../../stores/settings'
import { useUiStore } from '../../../stores/ui'
import BaseButton from '../../ui/BaseButton.vue'
import BaseDialog from '../../ui/BaseDialog.vue'

// 初めてログインして ChatGPT プランの利用が有効になったときだけ出す確認（OpenAI の UI/UX ガイドライン）。
// 出すかどうかは Go が決める（アカウント＝クライアント登録ごとに永続。再ログインでは立たない）。「Got it」で確認済みにする。
const props = defineProps<{ accounts: ChatGPTAccount[] }>()

const store = useSettingsStore()
const ui = useUiStore()

// 記録に失敗しても、押したあとに出し続けない（記録できていなければ、次の読み込みでもう一度出る）。
const dismissed = ref(new Set<string>())
const target = computed(() => props.accounts.find((a) => a.planNoticePending && a.signedIn && !dismissed.value.has(a.id)))

async function gotIt(): Promise<void> {
  const a = target.value
  if (!a) return
  dismissed.value = new Set([...dismissed.value, a.id])
  try {
    await store.ackChatGPTPlanNotice(a.id)
  } catch {
    ui.pushToast('error', msg.ai.chatgpt.planNotice.ackFailed)
  }
}
</script>

<template>
  <BaseDialog :open="target !== undefined" :title="msg.ai.chatgpt.planNotice.title" size="sm" :closable="false">
    <div class="notice" data-testid="ai-chatgpt-plan-notice">
      <p class="text">{{ msg.ai.chatgpt.planNotice.body }}</p>
      <p v-if="target?.email" class="note">{{ msg.ai.chatgpt.planNotice.account(target.email) }}</p>
      <p class="note">{{ msg.ai.chatgpt.planNotice.note }}</p>
    </div>
    <template #footer>
      <BaseButton variant="primary" data-testid="ai-chatgpt-notice-gotit" @click="gotIt">
        {{ msg.ai.chatgpt.planNotice.gotIt }}
      </BaseButton>
    </template>
  </BaseDialog>
</template>

<style scoped>
.notice {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-2);
}

.text {
  margin: 0;
  font-size: var(--font-size-md);
  line-height: 1.6;
}

.note {
  margin: 0;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
</style>
