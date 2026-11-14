<script setup lang="ts">
// 画面の右下の端のマスコット（Windows の別ウィンドウ。main.ts が #mascot のときだけ描く。#372）。
// 押すとメインウィンドウを前面に出してエージェントのパネルを開く。✕（ホバー・フォーカスで出る）はこのマスコットだけ消す。
// ストア（Pinia）は使わない。小さなウィンドウなので、バインディングとイベントだけで足りる。
import '@fontsource-variable/jetbrains-mono/wght.css'
import '../styles/tokens.css'
import '../styles/reset.css'
import { IconoirProvider, Xmark } from '@iconoir/vue'
import {
  OpenAgentPanel,
  SetAgentMascot,
} from '../../bindings/github.com/ousiassllc/hitomado/internal/app/agentcore/agentservice'
import AgentMascot from '../components/shell/AgentMascot.vue'
import CountBadge from '../components/ui/CountBadge.vue'
import { call } from '../lib/api'
import { mascot } from '../lib/messages'
import { useAgentPending } from './useAgentPending'

const { pending } = useAgentPending()

// 失敗は call がログへ報告する。マスコットには出す場所が無いので、ここでは何もしない。
function open(): void {
  call(OpenAgentPanel()).catch(() => undefined)
}

function hide(): void {
  call(SetAgentMascot(false)).catch(() => undefined)
}
</script>

<template>
  <IconoirProvider :icon-props="{ color: 'currentColor', strokeWidth: 1.5 }">
    <div class="wrap">
      <button
        type="button"
        class="open"
        :aria-label="mascot.openLabel(pending)"
        data-testid="mascot-open"
        @click="open"
      >
        <AgentMascot :size="40" />
        <!-- 名前（読み上げ）はボタンの aria-label に件数を含めてある。バッジは見た目だけ。 -->
        <span class="badge" aria-hidden="true"><CountBadge :count="pending" :label="mascot.pendingLabel(pending)" /></span>
      </button>
      <button type="button" class="hide" :aria-label="mascot.hideLabel" :title="mascot.hideLabel" data-testid="mascot-hide" @click="hide">
        <Xmark class="icon icon-sm" aria-hidden="true" />
      </button>
    </div>
  </IconoirProvider>
</template>

<style>
/* ウィンドウは透過。下の画面が見えるよう、通常の body の地（--bone）を消す（reset.css より強いセレクタで、読み込み順に依らない）。 */
html,
html body,
html body #app {
  background: transparent;
  overflow: hidden;
  color-scheme: light dark;
}
</style>

<style scoped>
/* ウィンドウは 72×72（Go の MascotSize）。丸いボタンを 8px 内側に置き、フォーカスの輪郭・バッジ・✕がウィンドウの外へ切れないようにする。 */
.wrap {
  position: relative;
  width: 100%;
  height: 100%;
}

.open {
  position: absolute;
  inset: var(--s-2);
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--border);
  border-radius: var(--r-full);
  background: var(--surface);
  color: var(--ink);
  cursor: pointer;
}

.open:hover {
  background: var(--accent-soft);
}

.badge {
  position: absolute;
  top: calc(-1 * var(--s-1));
  right: calc(-1 * var(--s-2));
}

.hide {
  position: absolute;
  top: var(--s-1);
  left: var(--s-1);
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  padding: 0;
  border: 1px solid var(--border);
  border-radius: var(--r-full);
  background: var(--surface);
  color: var(--ink);
  cursor: pointer;
  opacity: 0;
  transition: opacity var(--motion-base) var(--ease-out);
}

.wrap:hover .hide,
.wrap:focus-within .hide {
  opacity: 1;
}

.hide:hover {
  background: var(--accent-soft);
}
</style>
