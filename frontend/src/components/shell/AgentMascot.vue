<script setup lang="ts">
// エージェントのマスコット（いろはの案Aのお試し絵 20261114-hitomado-nav-mock の描き方。ストロークは currentColor・ガイドの色だけ）。装飾なので読み上げない。
defineProps<{ size?: number }>()
</script>

<template>
  <svg :width="size ?? 32" :height="size ?? 32" viewBox="0 0 40 40" aria-hidden="true" focusable="false" class="mascot">
    <path d="M20 9.5c-1.2-3 .6-5.4 3.6-5.6-.2 2.9-1.9 4.6-3.6 5.6z" class="leaf" stroke-width="1.4" stroke-linejoin="round" />
    <path d="M20 9.5v1.5" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" />
    <path d="M6 25c0-8.3 6.3-14.5 14-14.5S34 16.7 34 25c0 6.4-5.6 10-14 10S6 31.4 6 25z" class="face" stroke-width="1.6" />
    <circle cx="15" cy="24" r="1.9" fill="currentColor" />
    <circle cx="25" cy="24" r="1.9" fill="currentColor" />
    <path d="M17.6 28.2c1.4 1.3 3.4 1.3 4.8 0" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
  </svg>
</template>

<style scoped>
.mascot {
  flex: none;
  color: var(--ink);
}

.leaf {
  fill: var(--accent-soft);
  stroke: currentColor;
}

.face {
  fill: var(--surface);
  stroke: currentColor;
}
</style>
