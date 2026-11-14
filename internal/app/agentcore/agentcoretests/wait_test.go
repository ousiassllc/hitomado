package agentcoretests

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// drain は、これまでに積んだ新着の処理が済んだことを保証する。末尾に番兵の新着（必ず 1 件の項目になる。アカウント b の
// 新しいスレッド）を積み、その項目ができるのを待つ（待ち行列は FIFO で、ワーカーは 1 つなので、前の分は処理済み）。
// 番兵の項目と、判断・下書きの 1 回分は、そのあとの検証に含まれる。固定時間の待ちは使わない。
func (f *fixture) drain(t *testing.T) {
	t.Helper()
	f.sentinels++
	id := fmt.Sprintf("sentinel-%d", f.sentinels)
	f.putInbound(t, f.b, id, "m-"+id)
	before := len(f.items(t).Items)
	f.newMail(f.b.ID, id, "m-"+id)
	f.waitItems(t, before+1)
}

// waitLog はログに substr が n 回以上出るまで待つ（上限つきの条件待ち）。
func (f *fixture) waitLog(t *testing.T, substr string, n int) {
	t.Helper()
	require.Eventually(t, func() bool { return strings.Count(f.logs.String(), substr) >= n },
		waitTimeout, 2*time.Millisecond, "ログ %q を %d 回待てなかった", substr, n)
}
