<script setup lang="ts">
import { computed } from 'vue'
import type { AIModelList } from '../../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import { list } from '../../../lib/api'
import { settings as msg } from '../../../lib/messages'
import BaseButton from '../../ui/BaseButton.vue'
import BaseSelect, { type SelectOption } from '../../ui/BaseSelect.vue'
import SettingsInput from '../SettingsInput.vue'

// モデル欄。取得した一覧があれば一覧から選び、無い（取得できない・まだ取得していない）ときは手入力。
// 保存済みのモデルが一覧に無くても消さず、選択肢に残す。
const props = defineProps<{
  modelValue: string
  /** 最後に取得した結果（未取得なら undefined）。 */
  models?: AIModelList
  loading: boolean
  /** 「一覧を更新」を押せるか（ほかの処理中でないとき。選んだ送信先は保存前でも取得できる）。 */
  canRefresh: boolean
  error?: string
}>()
const emit = defineEmits<{ 'update:modelValue': [value: string]; refresh: []; enter: [] }>()

// Go の nil スライスは null で届き得るので、常に配列として扱う。
const listed = computed(() => list(props.models?.models))

const options = computed<SelectOption[]>(() => {
  const out = listed.value.map((m) => ({
    value: m.id,
    label: m.displayName && m.displayName !== m.id ? `${m.displayName}（${m.id}）` : m.id,
  }))
  const current = props.modelValue.trim()
  if (current !== '' && !listed.value.some((m) => m.id === current)) {
    out.unshift({ value: current, label: msg.ai.modelNotListed(current) })
  }
  return out
})

const picking = computed(() => listed.value.length > 0)
const note = computed(() => {
  const m = props.models
  if (m === undefined) return undefined
  if (listed.value.length > 0) return msg.ai.modelsLoaded(listed.value.length)
  return m.message || undefined
})
</script>

<template>
  <div class="model" data-testid="ai-model">
    <div v-if="picking" class="field">
      <span class="label">{{ msg.ai.model }}</span>
      <BaseSelect
        :model-value="modelValue"
        :options="options"
        :placeholder="msg.ai.modelSelectPlaceholder"
        :ariaLabel="msg.ai.model"
        @update:model-value="(v) => emit('update:modelValue', v)"
      />
      <p v-if="error" class="error" role="alert">{{ error }}</p>
    </div>
    <SettingsInput
      v-else
      :model-value="modelValue"
      :label="msg.ai.model"
      :placeholder="msg.ai.modelPlaceholder"
      :error="error"
      :hint="modelValue.trim() === '' ? msg.ai.modelRequired : msg.ai.modelsManualHint"
      @update:model-value="(v) => emit('update:modelValue', v)"
      @enter="emit('enter')"
    />
    <div class="refresh">
      <BaseButton variant="secondary" :disabled="!canRefresh || loading" :loading="loading" @click="emit('refresh')">
        {{ loading ? msg.ai.modelsRefreshing : msg.ai.modelsRefresh }}
      </BaseButton>
      <p v-if="note" class="note" role="status" data-testid="ai-models-note">{{ note }}</p>
    </div>
  </div>
</template>

<style scoped>
.model {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

.field {
  display: flex;
  flex-direction: column;
  gap: var(--gap-label);
  max-width: 320px;
}

.label {
  font-size: var(--font-size-sm);
}

.refresh {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--s-2);
}

.note {
  margin: 0;
  color: var(--ink);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.error {
  margin: calc(var(--gap-help) - var(--gap-label)) 0 0;
  color: var(--danger);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}
</style>
