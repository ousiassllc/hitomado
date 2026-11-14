import { onBeforeUnmount, onMounted, onScopeDispose, ref } from 'vue'
import { ListAgentItems } from '../../bindings/github.com/ousiassllc/hitomado/internal/app/agentcore/agentservice'
import { call } from '../lib/api'
import { onEvent } from '../lib/events'

/** Go の events.AgentUpdated。マスコットは他のストアに依存しないので、ここで直接購読する。 */
const AGENT_UPDATED = 'agent:updated'

interface AgentUpdatedPayload {
  pendingCount: number
}

/**
 * エージェントの確認待ち・質問待ちの件数。起動時とウィンドウが再び見えたとき（隠れている間のイベントの取りこぼし）に
 * 一覧から読み、以後は agent:updated のペイロードで更新する。読み込みに失敗しても 0 のまま（マスコットは件数 0 でも出す）。
 */
export function useAgentPending() {
  const pending = ref(0)
  let updates = 0 // 読み込みの最中にイベントが届いたら、新しいほう（イベント）を残す

  async function load(): Promise<void> {
    const seen = updates
    try {
      const overview = await call(ListAgentItems())
      if (seen === updates) pending.value = overview.pendingCount
    } catch {
      // 失敗は call がログへ報告済み。
    }
  }

  const off = onEvent<AgentUpdatedPayload>(AGENT_UPDATED, (p) => {
    updates++
    pending.value = p.pendingCount
  })
  onScopeDispose(off)

  function onVisible(): void {
    if (document.visibilityState === 'visible') void load()
  }
  onMounted(() => {
    void load()
    document.addEventListener('visibilitychange', onVisible)
  })
  onBeforeUnmount(() => document.removeEventListener('visibilitychange', onVisible))

  return { pending }
}
