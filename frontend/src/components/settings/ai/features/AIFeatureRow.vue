<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { effortChoices } from '../../../../lib/aiConfig'
import { parseBindingError } from '../../../../lib/api'
import { settings as msg } from '../../../../lib/messages'
import { useSettingsStore } from '../../../../stores/settings'
import BaseButton from '../../../ui/BaseButton.vue'
import BaseSelect from '../../../ui/BaseSelect.vue'
import SettingsInput from '../../SettingsInput.vue'
import AIEffortField from '../AIEffortField.vue'
import AIStatusChip from '../providers/AIStatusChip.vue'
import { isConnected } from '../providers/aiProviders'
import {
  type AIFeatureId,
  featureChoices,
  isJevOnly,
  modelListFor,
  optionValue,
  parseOption,
} from './aiFeatures'

// 「機能ごとに使う AI」の 1 行（#376）。つないである AI のモデルだけを選べ、選ぶとすぐ保存する。
// 選んでいた AI が外された・ログインが切れたときは、選択を残したまま「つなぎ直しが必要」とつなぎ直しの入口を出す（別の AI へは切り替えない）。
// 「AI で下書き」だけ、モデルの横にエフォートを置く。モデル一覧を取得できないプロバイダは、モデル名を手入力する。
const props = defineProps<{ feature: AIFeatureId; label: string }>()
/** connect: AI をつなぐ（provider なし）／つなぎ直す（そのプロバイダのページへ）。 */
const emit = defineEmits<{ connect: [provider?: string] }>()
const store = useSettingsStore()
const t = msg.ai.use

const cfg = computed(() => store.aiConfig)
const saved = computed(() => cfg.value?.features[props.feature] ?? { provider: '', model: '' })
const choices = computed(() =>
  cfg.value
    ? featureChoices(props.feature, cfg.value, store.aiModels, saved.value)
    : { options: [], manual: [], loading: false, noneAvailable: true },
)
const hasSaved = computed(() => saved.value.provider !== '')
const needsReconnect = computed(() => hasSaved.value && !isConnected(saved.value.provider, cfg.value))
const empty = computed(() => choices.value.noneAvailable && !hasSaved.value)

/** Inbox Zero で選べる AI が TypeSafe AI（Jev）だけのときは固定表示にする（選択はここで保存する）。 */
const fixedJev = computed(() => props.feature === 'inboxZero' && isJevOnly(choices.value))

const busy = ref(false)
const error = ref<string>()
const typed = ref<Record<string, string>>({})

const modelList = (provider: string) => (cfg.value ? modelListFor(provider, cfg.value, store.aiModels) : undefined)
const efforts = computed(() =>
  props.feature === 'draft' && hasSaved.value
    ? effortChoices(saved.value.provider, saved.value.model, modelList(saved.value.provider))
    : [],
)

async function save(provider: string, model: string, effort = ''): Promise<void> {
  if (busy.value) return
  error.value = undefined
  busy.value = true
  try {
    await store.setAIFeature(props.feature, provider, model, effort)
  } catch (e) {
    error.value = parseBindingError(e).message
  } finally {
    busy.value = false
  }
}

/** 選び直したモデルでも、そのまま選べるエフォートは引き継ぐ（無いときは「モデルの既定」へ戻す。黙って別の値にしない）。 */
function onChoose(value: string): void {
  const { provider, model } = parseOption(value)
  const keep = props.feature === 'draft' ? saved.value.effort ?? '' : ''
  const next = effortChoices(provider, model, modelList(provider))
  void save(provider, model, next.includes(keep) ? keep : '')
}

watch(
  fixedJev,
  (fixed) => {
    if (fixed && !hasSaved.value) void save('typesafe', '')
  },
  { immediate: true },
)

function manualValue(provider: string): string {
  return typed.value[provider] ?? (saved.value.provider === provider ? saved.value.model : '')
}

function saveManual(provider: string): void {
  const model = manualValue(provider).trim()
  if (model === '') {
    error.value = msg.ai.modelRequired
    return
  }
  const keep = provider === saved.value.provider && model === saved.value.model ? saved.value.effort ?? '' : ''
  void save(provider, model, keep)
}
</script>

<template>
  <li class="frow" :data-testid="`ai-feature-${feature}`" :data-state="empty ? 'empty' : needsReconnect ? 'reconnect' : 'ready'">
    <div class="flab">{{ label }}</div>
    <div class="fctrls">
      <p v-if="empty" class="fmsg" data-testid="ai-feature-empty">
        {{ t.noneAvailable }}
        <BaseButton variant="ghost" size="sm" class="link" :data-testid="`ai-feature-connect-${feature}`" @click="emit('connect')">
          {{ t.connect }}
        </BaseButton>
      </p>
      <template v-else>
        <div class="need">
          <AIStatusChip v-if="needsReconnect" kind="warn" data-testid="ai-feature-reconnect">{{ t.reconnect }}</AIStatusChip>
          <span v-if="fixedJev" class="fixed" data-testid="ai-feature-fixed">{{ t.fixed(msg.ai.providerList.names.typesafe ?? '') }}</span>
          <div v-else class="pick">
            <BaseSelect
              :model-value="hasSaved ? optionValue(saved.provider, saved.model) : undefined"
              :options="choices.options"
              :placeholder="t.placeholder"
              :ariaLabel="t.selectLabel(label)"
              @update:model-value="onChoose"
            />
          </div>
          <BaseButton
            v-if="needsReconnect"
            variant="ghost"
            size="sm"
            class="link"
            :data-testid="`ai-feature-fix-${feature}`"
            @click="emit('connect', saved.provider)"
          >
            {{ saved.provider === 'chatgpt' ? t.relogin : t.fixConnection }}
          </BaseButton>
        </div>
        <AIEffortField
          v-if="feature === 'draft' && !needsReconnect"
          :efforts="efforts"
          :model-value="saved.effort ?? ''"
          @update:model-value="(e) => save(saved.provider, saved.model, e)"
        />
        <p v-if="choices.loading" class="note" role="status">{{ t.loading }}</p>
        <form
          v-for="m in choices.manual"
          :key="m.provider"
          class="manual"
          :data-testid="`ai-feature-manual-${feature}-${m.provider}`"
          @submit.prevent="saveManual(m.provider)"
        >
          <SettingsInput
            mono
            :model-value="manualValue(m.provider)"
            :label="t.manualLabel(msg.ai.providerList.names[m.provider] ?? m.provider)"
            :placeholder="msg.ai.modelPlaceholder"
            :hint="m.message ?? msg.ai.modelsManualHint"
            @update:model-value="(v) => (typed[m.provider] = v)"
            @enter="saveManual(m.provider)"
          />
          <BaseButton variant="secondary" size="sm" type="submit" :loading="busy">{{ t.manualSave }}</BaseButton>
        </form>
        <p v-if="error" class="error" role="alert">{{ error }}</p>
      </template>
    </div>
  </li>
</template>

<style scoped>
.frow {
  display: flex;
  align-items: flex-start;
  gap: var(--s-3);
  min-height: 48px;
  padding: var(--s-2) var(--s-4);
}

.flab {
  display: flex;
  flex: none;
  align-items: center;
  width: 148px;
  min-height: var(--control-h);
  font-weight: 500;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.fctrls {
  display: flex;
  flex: 1;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-2);
  min-width: 0;
}

.fmsg {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--s-3);
  margin: 0;
  min-height: var(--control-h);
  color: var(--mute);
  font-size: var(--font-size-sm);
}

.need {
  display: flex;
  flex: 1;
  align-items: center;
  gap: var(--s-2);
  min-width: 0;
}

.pick {
  display: flex;
  flex: 1;
  max-width: 320px;
  min-width: 180px;
}

.pick :deep(.sel) {
  flex: 1;
  min-width: 0;
}

.fixed {
  display: inline-flex;
  align-items: center;
  min-height: var(--control-h);
  font-size: var(--font-size-sm);
}

.link {
  min-height: 0;
  padding: 0;
  border-color: transparent;
  font-weight: 500;
  white-space: nowrap;
  text-decoration: underline;
  text-underline-offset: 2px;
}

.note,
.error {
  flex-basis: 100%;
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.note {
  color: var(--mute);
}

.error {
  color: var(--danger);
}

.manual {
  display: flex;
  flex-basis: 100%;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: var(--s-2);
}

.manual :deep(.field) {
  flex: 1;
  max-width: 320px;
  min-width: 180px;
}

/* 390px 幅: 行はカードになり、ラベルを上に置いて、選択欄は横いっぱいにする。 */
@media (max-width: 640px) {
  .frow {
    flex-direction: column;
    align-items: stretch;
    gap: var(--s-2);
    min-height: 0;
    padding: var(--s-3);
    line-break: strict;
    word-break: keep-all;
  }

  .flab {
    width: auto;
    min-height: 0;
  }

  .fctrls {
    flex-direction: column;
    align-items: stretch;
  }

  .need {
    flex-direction: column;
    align-items: stretch;
  }

  .pick {
    max-width: none;
    min-width: 0;
  }

  .manual :deep(.field) {
    max-width: none;
  }
}
</style>
