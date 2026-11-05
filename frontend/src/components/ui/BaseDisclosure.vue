<script setup lang="ts">
/** 折りたたみ。初回だけ必要な補足説明を、既定では閉じて置く（ネイティブの <details>）。 */
defineProps<{ summary: string }>()
</script>

<template>
  <details class="disclosure">
    <summary class="disclosure-summary">{{ summary }}</summary>
    <div class="disclosure-body"><slot /></div>
  </details>
</template>

<style scoped>
/* ガイドの P1「アコーディオン」: 枠つきカード＋右端のシェブロン（ネイティブのマーカーは消す） */
.disclosure {
  border: 1px solid var(--border);
  border-radius: var(--r-lg);
  background: var(--surface);
  overflow: hidden;
}

.disclosure-summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--s-3);
  padding: var(--s-3) var(--s-4);
  color: var(--ink);
  font-weight: 600;
  font-size: var(--font-size-sm);
  list-style: none;
  cursor: pointer;
}

.disclosure-summary::-webkit-details-marker {
  display: none;
}

.disclosure-summary::after {
  content: "";
  flex: none;
  width: 8px;
  height: 8px;
  border-right: 1.5px solid var(--mute);
  border-bottom: 1.5px solid var(--mute);
  transform: rotate(45deg);
  transition: transform var(--motion-base) var(--ease-out);
}

.disclosure[open] > .disclosure-summary::after {
  transform: rotate(-135deg);
}

/* 角丸と overflow: hidden で外側の outline が切れるので内側に出す */
.disclosure-summary:focus-visible {
  outline: 2px solid var(--focus);
  outline-offset: -2px;
}

.disclosure-body {
  padding: 0 var(--s-4) var(--s-3);
  color: var(--mute);
}
</style>
