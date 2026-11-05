<script setup lang="ts">
import { EditorContent, useEditor } from '@tiptap/vue-3'
import { onBeforeUnmount, ref, watch } from 'vue'
import { compose as msg } from '../../lib/messages'
import { cleanRichHtml } from '../../lib/richtext'
import { ensureLeadingParagraph } from '../../lib/signature'
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
      <button
        type="button"
        class="rte-btn rte-bold"
        :class="{ 'rte-btn-on': editor?.isActive('bold') }"
        :aria-pressed="editor?.isActive('bold') ?? false"
        :aria-label="msg.editor.bold"
        :title="msg.editor.bold"
        @click="editor?.chain().focus().toggleBold().run()"
      >
        B
      </button>
      <button
        type="button"
        class="rte-btn"
        :class="{ 'rte-btn-on': editor?.isActive('link') }"
        :aria-pressed="editor?.isActive('link') ?? false"
        :aria-expanded="linkOpen"
        :aria-label="msg.editor.link"
        :title="msg.editor.link"
        @click="toggleLink"
      >
        🔗
      </button>
      <button
        type="button"
        class="rte-btn"
        :class="{ 'rte-btn-on': editor?.isActive('bulletList') }"
        :aria-pressed="editor?.isActive('bulletList') ?? false"
        :aria-label="msg.editor.bulletList"
        :title="msg.editor.bulletList"
        @click="editor?.chain().focus().toggleBulletList().run()"
      >
        •≡
      </button>
    </div>
    <EditorLinkForm
      v-if="linkOpen"
      :initial="linkInitial"
      @apply="applyLink"
      @remove="removeLink"
      @cancel="closeLink"
    />
    <EditorContent :editor="editor" class="rte-body" />
  </div>
</template>

<style scoped>
.rte {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-bg);
  color: var(--color-text);
}

.rte:focus-within {
  border-color: var(--color-focus);
}

.rte-disabled {
  opacity: 0.7;
}

.rte-toolbar {
  display: flex;
  gap: var(--space-1);
  padding: var(--space-1) var(--space-2);
  border-bottom: 1px solid var(--color-border);
  background: var(--color-bg-subtle);
}

.rte-btn {
  min-width: 32px;
  height: 28px;
  padding: 0 var(--space-2);
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text);
  font-size: var(--font-size-md);
  cursor: pointer;
}

.rte-bold {
  font-weight: 700;
}

.rte-btn:hover {
  background: var(--color-selected);
}

.rte-btn-on {
  border-color: var(--color-accent);
  background: var(--color-selected);
}

.rte-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.rte-body :deep(.editor-content) {
  min-height: 120px;
  padding: var(--space-3);
  outline: none;
  font-size: var(--font-size-md);
  line-height: 1.6;
  overflow-wrap: anywhere;
}

.rte-body :deep(.editor-content p) {
  margin: 0 0 var(--space-2);
}

.rte-body :deep(.editor-content ul) {
  margin: 0 0 var(--space-2);
  padding-left: var(--space-6);
}

.rte-body :deep(.editor-content li > p) {
  margin: 0;
}

.rte-body :deep(.editor-content a) {
  color: var(--color-accent);
  text-decoration: underline;
}

.rte-body :deep([data-hitomado-signature]) {
  margin-top: var(--space-3);
  color: var(--color-text-muted);
}

.rte-body :deep([data-hitomado-quote]) {
  margin: var(--space-3) 0 0;
  padding-left: var(--space-3);
  border-left: 3px solid var(--color-border);
  color: var(--color-text-muted);
}
</style>
