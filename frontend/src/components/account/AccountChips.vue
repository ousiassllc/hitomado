<script setup lang="ts">
import { useAccountFilter } from '../../composables/useAccountFilter'
import { nav } from '../../lib/messages'
import { accountColorVar } from '../../lib/palette'
import { useAccountsStore } from '../../stores/accounts'

// スマホのアカウントの絞り込み。検索欄の下に横一列のチップで並べる（サイドバーの「アカウント」と同じ軸・同じ操作）。
// 先頭は「全アカウント」（カテゴリタブの「すべて」と見分けるため）。選んだチップは地が --accent-soft。識別色は沈めた tint の点だけで、文字・塗りには使わない。
const accounts = useAccountsStore()
const { selectedIds, allSelected, toggle, selectAll } = useAccountFilter()
</script>

<template>
  <ul v-if="accounts.accounts.length > 0" class="chips" :aria-label="nav.accounts.heading" data-testid="account-chips">
    <li>
      <button type="button" class="chip" :aria-pressed="allSelected" @click="selectAll">{{ nav.accounts.all }}</button>
    </li>
    <li v-for="a in accounts.accounts" :key="a.id">
      <button type="button" class="chip" :aria-pressed="selectedIds.has(a.id)" :title="a.email" @click="toggle(a.id, true)">
        <span class="dot" :style="{ '--chip-tint': accountColorVar(a.color) }" aria-hidden="true" />
        <span class="name">{{ a.displayName }}</span>
      </button>
    </li>
  </ul>
</template>

<style scoped>
.chips {
  display: flex;
  gap: var(--s-2);
  margin: 0;
  padding: var(--s-1) var(--s-4) var(--s-2);
  list-style: none;
  overflow-x: auto;
}

.chips li {
  flex: none;
}

.chip {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: var(--s-2);
  box-sizing: border-box;
  max-width: 200px;
  min-height: var(--control-h-sm);
  padding: 0 var(--s-3);
  border: 1px solid var(--border);
  border-radius: var(--r-full);
  background: var(--surface);
  color: var(--ink);
  font: inherit;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  white-space: nowrap;
  cursor: pointer;
}

/* 見た目は 32px のまま、当たり判定だけ縦に広げて 44px 以上にする（touch.css のスイッチと同じ）。 */
.chip::after {
  content: '';
  position: absolute;
  inset: -6px 0;
}

.chip[aria-pressed='true'] {
  border-color: transparent;
  background: var(--accent-soft);
}

.name {
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 識別色（アカウントを見分ける印）。一覧のチップと同じ沈め方（地 --tint-bg・枠 --tint-border）。 */
.dot {
  flex: none;
  width: 10px;
  height: 10px;
  border-radius: var(--r-full);
  background: color-mix(in srgb, var(--chip-tint) var(--tint-bg), var(--surface));
  border: 1px solid color-mix(in srgb, var(--chip-tint) 70%, transparent);
}
</style>
