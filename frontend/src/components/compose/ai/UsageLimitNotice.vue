<script setup lang="ts">
import { WarningTriangle } from '@iconoir/vue'
import { compose as msg, settings as settingsMsg } from '../../../lib/messages'
import BaseButton from '../../ui/BaseButton.vue'

// ChatGPT プランの利用枠を超えたときのコンパクトな案内（OpenAI の UI/UX ガイドライン: 利用枠の超過では
// Manage usage を主なアクションにする。ChatGPT であることが分かる見出しを残す）。ロゴは使わない（docs/requirements/functional.md F-08）。
defineEmits<{ manage: []; dismiss: [] }>()
</script>

<template>
  <section class="limit" role="alert" data-testid="ai-usage-limit">
    <WarningTriangle class="icon icon-sm icon-warning limit-icon" aria-hidden="true" />
    <div class="limit-main">
      <header class="identity">{{ msg.ai.usageLimit.identity }}</header>
      <h4 class="title">{{ msg.ai.usageLimit.title }}</h4>
      <p class="body">{{ msg.ai.usageLimit.body }}</p>
      <p class="note">{{ msg.ai.usageLimit.note }}</p>
      <div class="actions">
        <BaseButton size="sm" variant="primary" data-testid="ai-usage-limit-manage" @click="$emit('manage')">
          {{ settingsMsg.ai.chatgpt.manageUsage }}
        </BaseButton>
        <BaseButton size="sm" variant="ghost" data-testid="ai-usage-limit-dismiss" @click="$emit('dismiss')">
          {{ msg.ai.usageLimit.dismiss }}
        </BaseButton>
      </div>
    </div>
  </section>
</template>

<style scoped>
.limit {
  display: flex;
  align-items: flex-start;
  gap: var(--s-2);
  padding: var(--s-2) var(--s-3);
  border: 1px solid var(--warning-border);
  border-radius: var(--r-md);
  background: var(--warning-soft);
  color: var(--ink);
}

.limit-icon {
  margin-top: var(--s-1);
}

.limit-main {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--s-1);
  min-width: 0;
}

.identity {
  color: var(--mute);
  font-size: var(--font-size-xs);
  line-height: var(--line-height-xs);
}

.title {
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.body,
.note {
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.note {
  color: var(--mute);
}

.actions {
  display: flex;
  gap: var(--s-2);
  margin-top: var(--s-1);
}
</style>
