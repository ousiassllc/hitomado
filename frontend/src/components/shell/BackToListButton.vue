<script setup lang="ts">
import { ArrowLeft } from '@iconoir/vue'
import { common } from '../../lib/messages'
import { useInboxStore } from '../../stores/inbox'
import { useLayoutStore } from '../../stores/layout'
import BaseIconButton from '../ui/BaseIconButton.vue'

const inbox = useInboxStore()
const layout = useLayoutStore()
</script>

<template>
  <!-- 1 画面ずつのレイアウトでスレッドを閉じて一覧へ戻る（Android の戻るボタンと同じ。2 ペイン・3 ペインでは一覧が見えているので出さない） -->
  <BaseIconButton
    v-if="layout.isStack"
    :label="common.shell.backToList"
    data-testid="back-to-list"
    @click="inbox.select(undefined)"
  >
    <ArrowLeft class="icon" aria-hidden="true" />
  </BaseIconButton>
</template>
