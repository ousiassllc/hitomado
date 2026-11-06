<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { ChatGPTAccount } from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { settings as msg } from '../../../lib/messages'
import { useSettingsStore } from '../../../stores/settings'
import { useUiStore } from '../../../stores/ui'
import BaseButton from '../../ui/BaseButton.vue'
import BaseDialog from '../../ui/BaseDialog.vue'

// 初めてログインして ChatGPT プランの利用が有効になったときだけ出す確認（OpenAI の UI/UX ガイドライン）。
// 出すかどうかは Go が決める（アカウント＝クライアント登録ごとに永続。再ログインでは立たない）。「Got it」を押したときだけ確認済みにする（Esc・外側クリックでは閉じない）。
const props = defineProps<{ accounts: ChatGPTAccount[] }>()

const store = useSettingsStore()
const ui = useUiStore()

// 記録に失敗しても、押したあとに出し続けない（記録できていなければ、次の読み込みでもう一度出る）。
const dismissed = ref(new Set<string>())
const target = computed(() => props.accounts.find((a) => a.planNoticePending && a.signedIn && !dismissed.value.has(a.id)))
// 確認済みで target が無くなっても、退出のフェード中はメールアドレスの行を残す（ダイアログが縮まないように、最後の対象を保持する）。
const shown = ref(target.value)
watch(target, (a) => {
  if (a) shown.value = a
})

async function gotIt(): Promise<void> {
  const a = target.value
  if (!a) return
  dismissed.value = new Set([...dismissed.value, a.id])
  try {
    await store.ackChatGPTPlanNotice(a.id)
  } catch {
    ui.pushAlert(msg.ai.chatgpt.planNotice.ackFailed)
  }
}
</script>

<template>
  <BaseDialog :open="target !== undefined" :title="msg.ai.chatgpt.planNotice.title" size="sm" :closable="false">
    <div class="notice" data-testid="ai-chatgpt-plan-notice">
      <p class="text">{{ msg.ai.chatgpt.planNotice.body }}</p>
      <p v-if="shown?.email" class="note">{{ msg.ai.chatgpt.planNotice.account(shown.email) }}</p>
      <p class="text">{{ msg.ai.chatgpt.planNotice.note }}</p>
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
  gap: var(--s-2);
}

.text {
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.note {
  margin: 0;
  color: var(--mute);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}
</style>
