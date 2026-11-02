<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { thread as msg } from '../../lib/messages'
import { buildSrcdoc, sanitizeEmailHtml } from '../../lib/sanitize'
import BaseButton from '../ui/BaseButton.vue'
import { bindFrameDocument, FALLBACK_HEIGHT } from './bodyFrame'

const props = defineProps<{ html?: string; text?: string; hasRemoteImages: boolean }>()
const emit = defineEmits<{ link: [url: string] }>()

/** このメッセージだけ画像を表示するか（メッセージ単位。親が key でメッセージごとに保持する） */
const showImages = ref(false)

// リモート画像の復元は、ブロック前の HTML を保持しているのでそのまま再サニタイズして行う。
const srcdoc = computed(() =>
  props.html
    ? buildSrcdoc(sanitizeEmailHtml(props.html, { allowRemoteImages: showImages.value }), { allowImages: showImages.value })
    : '',
)

/** 計測した高さ。undefined は未計測または計測不能で、固定高＋内部スクロールにする。 */
const height = ref<number>()
const frame = ref<HTMLIFrameElement>()
const unbind = shallowRef<() => void>()

function release(): void {
  unbind.value?.()
  unbind.value = undefined
}

function onLoad(): void {
  release()
  height.value = undefined
  const doc = (() => {
    try {
      return frame.value?.contentDocument
    } catch {
      return null
    }
  })()
  unbind.value = bindFrameDocument(doc, {
    onHeight: (px) => {
      height.value = px
    },
    onLink: (url) => emit('link', url),
  })
}

watch(srcdoc, () => {
  release()
  height.value = undefined
})
onBeforeUnmount(release)

const frameStyle = computed(() => ({ height: `${height.value ?? FALLBACK_HEIGHT}px` }))
</script>

<template>
  <div class="message-body">
    <div v-if="html && hasRemoteImages && !showImages" class="image-bar" data-testid="image-bar">
      <span>{{ msg.imagesBlocked }}</span>
      <BaseButton data-testid="show-images" @click="showImages = true">{{ msg.showImages }}</BaseButton>
    </div>
    <iframe
      v-if="html"
      ref="frame"
      class="body-frame"
      data-testid="body-frame"
      sandbox="allow-same-origin"
      referrerpolicy="no-referrer"
      :title="msg.bodyFrameTitle"
      :srcdoc="srcdoc"
      :style="frameStyle"
      @load="onLoad"
    ></iframe>
    <pre v-else-if="text" class="body-text" data-testid="body-text">{{ text }}</pre>
    <p v-else class="body-empty">{{ msg.noBody }}</p>
  </div>
</template>

<style scoped>
.message-body {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.image-bar {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  background: var(--color-warning-bg);
  color: var(--color-warning-text);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-sm);
}

.body-frame {
  display: block;
  width: 100%;
  border: 0;
  /* 本文の背景は srcdoc 側（color-scheme: light の Canvas）で描く。色値は直書きしない。 */
  border-radius: var(--radius-sm);
}

.body-text {
  margin: 0;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-family: var(--font-sans);
  font-size: var(--font-size-md);
  color: var(--color-text);
}

.body-empty {
  margin: 0;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
</style>
