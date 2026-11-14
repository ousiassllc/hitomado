<script setup lang="ts">
import { DoubleCheck } from '@iconoir/vue'
import { computed, onMounted } from 'vue'
import { inboxZero as msg } from '../../lib/messages'
import { vRipple } from '../../lib/ripple'
import { useInboxZeroStore } from '../../stores/inboxZero'
import { usePlatformStore } from '../../stores/platform'
import BaseTooltip from '../ui/BaseTooltip.vue'
import Spinner from '../ui/Spinner.vue'

// Inbox Zero ボタン（#314）。「すべて」タブの左に置く、アイコンだけの控えめな二次ボタン。
// 押せないとき（一覧が空・Inbox Zero に選んだ AI が無い／つながっていない・理由がすべてオフ）は、見た目は無効にするが、本物の disabled にはしない
// （disabled の要素にはツールチップも、タッチのタップも届かない）。ツールチップとタップ時のトーストで理由を出す。
// 判断中はスピナーを出し、二度押しできない。
const zero = useInboxZeroStore()
const platform = usePlatformStore()
onMounted(() => void zero.loadReasons())

const checking = computed(() => zero.phase === 'checking')
const blocked = computed(() => zero.blockedReason)
const label = computed(() =>
  blocked.value ? `${msg.label}（${blocked.value}）` : msg.label,
)

function onClick(): void {
  if (checking.value) return
  void zero.start()
}
</script>

<template>
  <BaseTooltip :text="blocked ?? msg.label">
    <button
      v-ripple
      class="zero-btn icon-btn"
      type="button"
      :class="{ blocked: !!blocked && !checking }"
      :aria-label="label"
      :aria-disabled="(!!blocked && !checking) || undefined"
      :aria-busy="checking || undefined"
      data-testid="inbox-zero"
      @click="onClick"
    >
      <Spinner v-if="checking" />
      <DoubleCheck v-else :class="platform.touchUi ? 'icon icon-lg' : 'icon'" aria-hidden="true" />
    </button>
  </BaseTooltip>
</template>

<style scoped>
.zero-btn {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: var(--control-h-sm);
  height: var(--control-h-sm);
  padding: 0;
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: transparent;
  color: var(--ink);
  cursor: pointer;
  /* ホバーは色・背景だけ */
  transition: background-color var(--motion-base) var(--ease-out);
}

.zero-btn:hover:not(.blocked):not([aria-busy='true']) {
  background: var(--accent-soft);
}

.zero-btn:focus-visible {
  outline: 2px solid var(--focus);
  outline-offset: 2px;
}

.zero-btn.blocked {
  opacity: 0.4;
  cursor: not-allowed;
}

.zero-btn[aria-busy='true'] {
  cursor: progress;
}
</style>
