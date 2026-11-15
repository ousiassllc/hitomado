<script setup lang="ts">
import type { Label } from '../../../bindings/github.com/ousiassllc/hitomado/internal/app/dto'
import BaseChip from '../ui/BaseChip.vue'
import BaseTooltip from '../ui/BaseTooltip.vue'

// Gmail のラベル色（ユーザー設定）は色相の出所にだけ使い、BaseChip が薄い地と枠に沈める（生の colorBg・colorFg は塗らない）。色の指定が無いラベルはトークンの既定色。
defineProps<{ label: Label }>()
</script>

<template>
  <BaseTooltip :text="label.name">
    <BaseChip :tint="label.colorBg || undefined">
      {{ label.name }}
    </BaseChip>
  </BaseTooltip>
</template>
