<script setup lang="ts">
import type { Label } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import BaseChip from '../ui/BaseChip.vue'

// Gmail のラベル色（ユーザー設定）を使う。色の指定が無いラベルはトークンの既定色。
defineProps<{ label: Label }>()
</script>

<template>
  <BaseChip :bg="label.colorBg || undefined" :fg="label.colorFg || undefined" :title="label.name">
    {{ label.name }}
  </BaseChip>
</template>
