<script setup lang="ts">
import { compose as msg, settings as settingsMsg } from '../../../lib/messages'
import BaseButton from '../../ui/BaseButton.vue'

// ChatGPT プランの利用枠を超えたときのコンパクトな案内（OpenAI の UI/UX ガイドライン: 利用枠の超過では
// Manage usage を主なアクションにする。ChatGPT であることが分かる見出しを残す）。ロゴは使わない（docs/requirements/functional.md F-08）。
defineEmits<{ manage: []; dismiss: [] }>()
</script>

<template>
  <section class="limit" role="alert" data-testid="ai-usage-limit">
    <header class="identity">{{ msg.ai.usageLimit.identity }}</header>
    <h4 class="title">{{ msg.ai.usageLimit.title }}</h4>
    <p class="body">{{ msg.ai.usageLimit.body }}</p>
    <p class="note">{{ msg.ai.usageLimit.note }}</p>
    <div class="actions">
      <BaseButton variant="primary" data-testid="ai-usage-limit-manage" @click="$emit('manage')">
        {{ settingsMsg.ai.chatgpt.manageUsage }}
      </BaseButton>
      <BaseButton variant="ghost" data-testid="ai-usage-limit-dismiss" @click="$emit('dismiss')">
        {{ msg.ai.usageLimit.dismiss }}
      </BaseButton>
    </div>
  </section>
</template>

<style scoped>
.limit {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-bg-elevated);
}

.identity {
  color: var(--color-text-muted);
  font-size: var(--font-size-xs);
  font-weight: 600;
}

.title {
  margin: 0;
  font-size: var(--font-size-md);
}

.body,
.note {
  margin: 0;
  font-size: var(--font-size-sm);
}

.note {
  color: var(--color-text-muted);
}

.actions {
  display: flex;
  gap: var(--space-2);
  margin-top: var(--space-1);
}
</style>
