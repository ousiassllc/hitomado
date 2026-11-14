<script setup lang="ts">
import { Bold, Link, List } from '@iconoir/vue'
import { EditorContent, useEditor } from '@tiptap/vue-3'
import { onBeforeUnmount, ref, watch } from 'vue'
import { compose as msg } from '../../lib/messages'
import { vOverlayScroll } from '../../lib/overlayScroll'
import { cleanRichHtml } from '../../lib/richtext'
import { ensureLeadingParagraph } from '../../lib/signature'
import BaseIconButton from '../ui/BaseIconButton.vue'
import EditorLinkForm from './EditorLinkForm.vue'
import { editorExtensions } from './editorExtensions'

const props = withDefaults(
  defineProps<{
    html: string
    /** false で閲覧のみ（送信確認中など）。 */
    editable?: boolean
    ariaLabel?: string
  }>(),
  { editable: true, ariaLabel: undefined },
)

const emit = defineEmits<{ 'update:html': [html: string] }>()

/** 最後に自分が発行した（または取り込んだ）HTML。props.html と一致するなら再設定しない。 */
let synced = props.html

const editor = useEditor({
  content: ensureLeadingParagraph(props.html),
  extensions: editorExtensions(),
  editable: props.editable,
  editorProps: {
    attributes: {
      role: 'textbox',
      'aria-multiline': 'true',
      'aria-label': props.ariaLabel ?? msg.editor.label,
      class: 'editor-content',
    },
    // 貼り付けは書式をクリーニング（プレーン＋許可タグのみ）。署名・引用の属性は貼り付けでは持ち込まない。
    transformPastedHTML: (html) => cleanRichHtml(html),
    // ファイルのドロップで webview がファイルへ遷移しないようにする（添付は AttachmentBar が受ける）。
    handleDrop: (_view, event) => (event.dataTransfer?.files?.length ?? 0) > 0,
  },
  onUpdate: ({ editor: ed }) => {
    synced = ed.getHTML()
    emit('update:html', synced)
  },
})

watch(
  () => props.html,
  (html) => {
    const ed = editor.value
    if (!ed || html === synced) return
    synced = html
    const { from, to } = ed.state.selection
    ed.commands.setContent(ensureLeadingParagraph(html), { emitUpdate: false })
    const max = ed.state.doc.content.size
    ed.commands.setTextSelection({ from: Math.min(from, max), to: Math.min(to, max) })
  },
)

watch(
  () => props.editable,
  // emitUpdate=false: 凍結（送信確認中など）の切替で本文が正規化されて update:html が発火し、確認が破棄されるのを防ぐ
  (v) => editor.value?.setEditable(v, false),
)

const linkOpen = ref(false)
const linkInitial = ref('')

function toggleLink(): void {
  const ed = editor.value
  if (!ed) return
  if (linkOpen.value) {
    closeLink()
    return
  }
  linkInitial.value = (ed.getAttributes('link').href as string | undefined) ?? ''
  linkOpen.value = true
}

function closeLink(): void {
  linkOpen.value = false
  editor.value?.chain().focus().run()
}

function applyLink(href: string): void {
  const ed = editor.value
  if (!ed) return
  linkOpen.value = false
  const chain = ed.chain().focus().extendMarkRange('link')
  if (ed.state.selection.empty && !ed.isActive('link')) {
    chain.insertContent({ type: 'text', text: href, marks: [{ type: 'link', attrs: { href } }] }).run()
  } else {
    chain.setLink({ href }).run()
  }
}

function removeLink(): void {
  linkOpen.value = false
  editor.value?.chain().focus().extendMarkRange('link').unsetLink().run()
}

onBeforeUnmount(() => editor.value?.destroy())
</script>

<template>
  <div class="rte" :class="{ 'rte-disabled': !editable }">
    <div v-if="editable" class="rte-toolbar" role="toolbar" :aria-label="msg.editor.toolbar">
      <BaseIconButton
        :class="{ 'rte-btn-on': editor?.isActive('bold') }"
        :aria-pressed="editor?.isActive('bold') ?? false"
        :label="msg.editor.bold"
        @click="editor?.chain().focus().toggleBold().run()"
      >
        <Bold class="icon icon-sm" aria-hidden="true" />
      </BaseIconButton>
      <BaseIconButton
        :class="{ 'rte-btn-on': editor?.isActive('link') }"
        :aria-pressed="editor?.isActive('link') ?? false"
        :aria-expanded="linkOpen"
        :label="msg.editor.link"
        @click="toggleLink"
      >
        <Link class="icon icon-sm" aria-hidden="true" />
      </BaseIconButton>
      <BaseIconButton
        :class="{ 'rte-btn-on': editor?.isActive('bulletList') }"
        :aria-pressed="editor?.isActive('bulletList') ?? false"
        :label="msg.editor.bulletList"
        @click="editor?.chain().focus().toggleBulletList().run()"
      >
        <List class="icon icon-sm" aria-hidden="true" />
      </BaseIconButton>
    </div>
    <Transition name="link-form">
      <EditorLinkForm
        v-if="linkOpen"
        :initial="linkInitial"
        @apply="applyLink"
        @remove="removeLink"
        @cancel="closeLink"
      />
    </Transition>
    <EditorContent v-overlay-scroll :editor="editor" class="rte-body" />
  </div>
</template>

<style scoped>
.rte {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  color: var(--ink);
}

.rte:focus-within {
  border-color: var(--focus);
  outline: 2px solid var(--focus);
  outline-offset: -1px;
}

.rte-disabled {
  opacity: 0.5;
}

.rte-toolbar {
  display: flex;
  gap: var(--s-1);
  padding: var(--s-1) var(--s-2);
  border-bottom: 1px solid var(--border);
  background: var(--bone);
}

/* 押されている（太字・リンク・箇条書きが今の位置で有効な）ボタン。BaseIconButton の hover 等より優先させる。 */
.rte-toolbar :deep(.rte-btn-on) {
  border-color: var(--accent);
  background: var(--accent-soft);
}

.rte-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.rte-body :deep(.editor-content) {
  min-height: 120px;
  padding: var(--s-3);
  outline: none;
  font-size: var(--font-size-base);
  line-height: var(--line-height-base);
  overflow-wrap: anywhere;
}

.rte-body :deep(.editor-content p) {
  margin: 0 0 var(--gap-para);
}

.rte-body :deep(.editor-content ul) {
  margin: 0 0 var(--gap-para);
  padding-left: var(--s-6);
}

/* 末尾の段落・箇条書きは下の余白を持たない。署名・引用（div / blockquote）の中の最後の p も含める（外へ突き抜けて本文末尾に 16px 残るため）。 */
.rte-body :deep(.editor-content :is(p, ul):last-child) {
  margin-bottom: 0;
}

.rte-body :deep(.editor-content li > p) {
  margin: 0;
}

.rte-body :deep(.editor-content a) {
  color: var(--ink);
  text-decoration: underline;
  text-decoration-color: var(--mute);
  text-underline-offset: 2px;
}

/* 利用者が付けた太字（<strong>）は reset.css の既定（400）に負けず、そのまま太字で見せる。 */
.rte-body :deep(.editor-content strong) {
  font-weight: 600;
}

.rte-body :deep([data-hitomado-signature]) {
  margin-top: var(--gap-para);
  color: var(--ink);
}

.rte-body :deep([data-hitomado-quote]) {
  margin: var(--gap-para) 0 0;
  padding-left: var(--s-3);
  border-left: 1px solid var(--mute);
  color: var(--ink);
}

/* 出現はフェードイン 320ms（ease-out）、退出はフェードアウト 240ms（ease-in）（M5。RecipientField の `.suggest-*` と同じ。移動なしで opacity だけ） */
.link-form-enter-active {
  animation: ui-fade-in var(--motion-fade-in) var(--ease-out);
}

.link-form-leave-active {
  animation: ui-fade-out var(--motion-fade-out) var(--ease-in) forwards;
}
</style>
