// DOMPurify を happy-dom 上で正しく動かすためのテスト専用の補正（実ブラウザでは不要）。
// 1. happy-dom の NodeIterator は、巡回中に現在ノードが取り除かれると以降を返さず終了する
//    （実ブラウザの NodeIterator は削除に追従する）。DOMPurify は巡回しながらノードを除去・置換する。
// 2. happy-dom の Node.prototype.nodeName は常に ''（サブクラスが上書きする）。DOMPurify は
//    Node.prototype の getter を直接呼ぶため、全要素が「許可されないタグ」になる。
// sanitize を最初に使う前に（例: テストファイルの先頭の import で）読み込むこと。

function following(node: Node, root: Node): Node | null {
  if (node.firstChild) return node.firstChild
  let n: Node | null = node
  while (n && n !== root) {
    if (n.nextSibling) return n.nextSibling
    n = n.parentNode
  }
  return null
}

/** 文書順で node の直前のノード（root の外には出ない）。 */
function preceding(node: Node, root: Node): Node | null {
  if (node === root) return null
  let n: Node | null = node.previousSibling
  if (!n) return node.parentNode
  while (n.lastChild) n = n.lastChild
  return n
}

class RemovalSafeNodeIterator {
  private started = false
  private last: Node | null = null
  private anchor: Node | null = null

  constructor(
    readonly root: Node,
    readonly whatToShow: number,
  ) {}

  private shown(n: Node): boolean {
    return (this.whatToShow & (1 << (n.nodeType - 1))) !== 0
  }

  nextNode(): Node | null {
    let candidate: Node | null
    if (!this.started) {
      this.started = true
      candidate = this.root
    } else if (this.last && this.root.contains(this.last)) {
      candidate = following(this.last, this.root)
    } else if (this.anchor) {
      // 現在ノードが取り除かれた: 直前のノードの次から続ける
      candidate = following(this.anchor, this.root)
    } else {
      return null
    }
    while (candidate && !this.shown(candidate)) candidate = following(candidate, this.root)
    this.last = candidate
    this.anchor = candidate ? preceding(candidate, this.root) : null
    return candidate
  }
}

function createNodeIterator(root: Node, whatToShow = 0xffffffff): unknown {
  return new RemovalSafeNodeIterator(root, whatToShow)
}

// createNodeIterator を実装している prototype（happy-dom の Document 階層のどこか）を差し替える。
for (let p = Object.getPrototypeOf(document); p; p = Object.getPrototypeOf(p)) {
  if (Object.hasOwn(p, 'createNodeIterator')) p.createNodeIterator = createNodeIterator
}

// Node.prototype.nodeName の getter を、最も派生した prototype の getter に委譲するものに差し替える。
const baseName = Object.getOwnPropertyDescriptor(Node.prototype, 'nodeName')
Object.defineProperty(Node.prototype, 'nodeName', {
  configurable: true,
  get(this: Node): string {
    for (let p = Object.getPrototypeOf(this); p && p !== Node.prototype; p = Object.getPrototypeOf(p)) {
      const d = Object.getOwnPropertyDescriptor(p, 'nodeName')
      if (d?.get) return d.get.call(this)
    }
    return baseName?.get?.call(this) ?? ''
  },
})

// DOMPurify が危険な要素（iframe・link）をパースする際、happy-dom が「読み込みは無効」と
// console.error に出す。サニタイズ対象として想定どおりの動きなので、この種のメッセージだけ捨てる。
const originalError = console.error.bind(console)
console.error = (...args: unknown[]): void => {
  const first = args[0]
  if (first instanceof Error && /^Failed to load (iframe page|external stylesheet)/.test(first.message)) return
  originalError(...args)
}
