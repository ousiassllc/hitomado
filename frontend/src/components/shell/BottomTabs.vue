<script setup lang="ts">
import { EditPencil, Mail, MoreHoriz } from '@iconoir/vue'
import { computed } from 'vue'
import { type Place, usePlaces } from '../../composables/usePlaces'
import { useUnreadTotal } from '../../composables/useUnreadTotal'
import { common, nav } from '../../lib/messages'
import { useInboxStore } from '../../stores/inbox'
import { useLayoutStore } from '../../stores/layout'
import { useUiStore } from '../../stores/ui'
import BaseMenu, { type MenuItem } from '../ui/BaseMenu.vue'
import CountBadge from '../ui/CountBadge.vue'
import AgentMascot from './AgentMascot.vue'

// スマホ（2 ペイン・1 画面ずつ）の下 4 タブ: 受信箱・下書き・エージェント・その他。ハンバーガーのドロワーの代わり。
// バッジは受信箱の未読数だけ（エージェントの確認待ちは、中身ができてから）。その他は 送信済み・アーカイブ・設定・アカウントを追加。
// 下書きは下書きの一覧（#366）。エージェントは中身ができるまでは「準備中」のパネルを開く。
const inbox = useInboxStore()
const layout = useLayoutStore()
const ui = useUiStore()
const unread = useUnreadTotal()
const places = usePlaces()

type Tab = 'inbox' | 'drafts' | 'agent' | 'more'

/** 選ばれているタブ。エージェントのパネルが開いていればそれ、検索中は無し、そうでなければ見る場所（送信済み・アーカイブは「その他」の中）。 */
const current = computed<Tab | undefined>(() => {
  if (ui.agentOpen) return 'agent'
  const v = places.current.value
  return v === undefined ? undefined : v === 'inbox' || v === 'drafts' ? v : 'more'
})

/** 場所を見せる。1 画面ずつのときは、開いているスレッドを閉じて一覧へ戻す（エージェントのパネルから戻るときも）。 */
function show(place: Place): void {
  ui.closeAgent()
  if (layout.isStack) inbox.select(undefined)
  places.show(place)
}

const moreItems: MenuItem[] = [
  { key: 'sent', label: nav.places.sent },
  { key: 'archive', label: nav.places.archive },
  { key: 'settings', label: common.sidebar.settings },
  { key: 'addAccount', label: common.sidebar.addAccount },
]

function onMore(key: string): void {
  if (key === 'sent' || key === 'archive') show(key)
  else if (key === 'settings') ui.openSettings()
  else if (key === 'addAccount') ui.openDialog('addAccount')
}
</script>

<template>
  <nav class="tabs" :aria-label="nav.tabs.label" data-testid="bottom-tabs">
    <button type="button" class="tab" :aria-current="current === 'inbox' ? 'page' : undefined" data-testid="tab-inbox" @click="show('inbox')">
      <span class="face">
        <Mail class="icon icon-md" aria-hidden="true" />
        <CountBadge class="pill" :count="unread" :label="common.sidebar.unread(unread)" />
      </span>
      {{ nav.tabs.inbox }}
    </button>
    <button type="button" class="tab" :aria-current="current === 'drafts' ? 'page' : undefined" data-testid="tab-drafts" @click="show('drafts')">
      <span class="face"><EditPencil class="icon icon-md" aria-hidden="true" /></span>
      {{ nav.tabs.drafts }}
    </button>
    <button type="button" class="tab" aria-controls="agent-panel" :aria-expanded="ui.agentOpen" data-testid="tab-agent" @click="ui.openAgent()">
      <span class="face"><AgentMascot :size="26" /></span>
      {{ nav.tabs.agent }}
    </button>
    <BaseMenu :items="moreItems" @select="onMore">
      <template #trigger>
        <button type="button" class="tab" :aria-current="current === 'more' ? 'page' : undefined" data-testid="tab-more">
          <span class="face"><MoreHoriz class="icon icon-md" aria-hidden="true" /></span>
          {{ nav.tabs.more }}
        </button>
      </template>
    </BaseMenu>
  </nav>
</template>

<style scoped>
.tabs {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  flex: none;
  min-height: var(--tabbar-h);
  padding-bottom: env(safe-area-inset-bottom, 0px);
  border-top: 1px solid var(--border);
  background: var(--surface);
}

.tab {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--s-1);
  min-width: 0;
  min-height: 44px;
  padding: var(--s-1) 0;
  border: none;
  background: transparent;
  color: var(--mute);
  font: inherit;
  font-size: var(--font-size-xs);
  line-height: var(--line-height-xs);
  cursor: pointer;
}

/* 選んでいるタブは --ink の文字と、アイコンの下地（--accent-soft のピル）。 */
.tab[aria-current='page'],
.tab[aria-expanded='true'] {
  color: var(--ink);
}

.face {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 30px;
  border-radius: var(--r-full);
}

.tab[aria-current='page'] .face,
.tab[aria-expanded='true'] .face {
  background: var(--accent-soft);
}

.pill {
  position: absolute;
  top: -6px;
  left: 30px;
}
</style>
