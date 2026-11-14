<script setup lang="ts">
import { Archive, EditPencil, Mail, Send } from '@iconoir/vue'
import { type Place, usePlaces } from '../../composables/usePlaces'
import { useUnreadTotal } from '../../composables/useUnreadTotal'
import { common, nav } from '../../lib/messages'
import CountBadge from '../ui/CountBadge.vue'

// サイドバーの「見る場所」。受信箱（未読数はここに付ける）・下書き・送信済み・アーカイブ。アカウントの絞り込みとは別の軸。
// 下書きは下書きの一覧（#366）を開く。件数は、一覧が件数を持ってから付ける。
const { current, show } = usePlaces()
const unread = useUnreadTotal()

const places = [
  { view: 'inbox', icon: Mail },
  { view: 'drafts', icon: EditPencil },
  { view: 'sent', icon: Send },
  { view: 'archive', icon: Archive },
] as const satisfies readonly { view: Place; icon: unknown }[]
</script>

<template>
  <nav class="places" :aria-label="nav.places.label">
    <button
      v-for="p in places"
      :key="p.view"
      type="button"
      class="place"
      :aria-current="current === p.view ? 'page' : undefined"
      :data-testid="`nav-${p.view}`"
      @click="show(p.view)"
    >
      <component :is="p.icon" class="icon icon-sm" aria-hidden="true" />
      <span class="label">{{ nav.places[p.view] }}</span>
      <CountBadge v-if="p.view === 'inbox'" :count="unread" :label="common.sidebar.unread(unread)" />
    </button>
  </nav>
</template>

<style scoped>
.places {
  display: flex;
  flex-direction: column;
  gap: var(--gap-menu);
  padding: 0 var(--s-2);
}

.place {
  display: flex;
  align-items: center;
  gap: var(--s-3);
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

.label {
  flex: 1;
}

.place:hover,
.place[aria-current='page'] {
  background: var(--accent-soft);
}
</style>
