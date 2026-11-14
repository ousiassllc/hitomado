<script setup lang="ts">
import { EditPencil, Mail } from '@iconoir/vue'
import { drafts as msg } from '../../lib/messages'
import { useDraftsStore } from '../../stores/drafts'
import { useUiStore } from '../../stores/ui'

// サイドバーの「受信箱」「下書き」（#366）。どちらを出すかは下書きの一覧を開いているか（drafts.active）で決まる。
// アカウントの選択（絞り込み）はどちらの一覧にも効く。ドロワーで開いているときは、選んだあと閉じて一覧を見せる。
const drafts = useDraftsStore()
const ui = useUiStore()

function choose(showDrafts: boolean): void {
  if (showDrafts) drafts.show()
  else drafts.hide()
  ui.closeMenu()
}
</script>

<template>
  <nav class="views" :aria-label="msg.nav.label">
    <button type="button" class="view" :aria-current="drafts.active ? undefined : 'page'" data-testid="nav-inbox" @click="choose(false)">
      <Mail class="icon icon-sm" aria-hidden="true" />{{ msg.nav.inbox }}
    </button>
    <button type="button" class="view" :aria-current="drafts.active ? 'page' : undefined" data-testid="nav-drafts" @click="choose(true)">
      <EditPencil class="icon icon-sm" aria-hidden="true" />{{ msg.nav.drafts }}
    </button>
  </nav>
</template>

<style scoped>
.views {
  display: flex;
  flex-direction: column;
  gap: var(--gap-menu);
  padding: 0 var(--s-2) var(--s-2);
}

.view {
  display: flex;
  align-items: center;
  gap: var(--s-2);
  width: 100%;
  min-height: var(--menu-item-h);
  padding: 0 var(--menu-pad-x);
  border: none;
  border-radius: var(--r-sm);
  background: transparent;
  color: var(--ink);
  font: inherit;
  font-size: var(--font-size-sm);
  text-align: left;
  cursor: pointer;
}

.view:hover,
.view[aria-current='page'] {
  background: var(--accent-soft);
}
</style>
