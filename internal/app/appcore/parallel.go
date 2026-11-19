package appcore

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/ousiassllc/nefos-inbox/internal/logging"
)

// SearchParallelism は 1 つの操作内で同時に走らせる Gmail 呼び出し数の上限（レート制限はクライアント側）。
// Gmail の利用上限はユーザーあたりの分単位の枠（threads.get は 1 回 10 units）で、同時数を増やしても枠は増えず、
// 当たったときに失敗する呼び出しが増えるだけなので、同期の取得（fetchConcurrency）と同じ 4 に揃える。
const SearchParallelism = 4

// ParallelDo は fn(0..n-1) を最大 limit 並列で実行し、すべての完了を待つ。
// fn は別 goroutine で動くため、panic は呼び出し元の recover に届かずプロセスを落とす。そこで fn ごとに捕捉して
// スタック付きで log へ記録し、ほかの fn は最後まで走らせる。panic を i 番目の失敗として扱いたい呼び出し側は、
// fn の中（の呼び出し先）で logging.RecoverTo を使ってエラーにする。ここは最後の安全網。
func ParallelDo(log *slog.Logger, n, limit int, fn func(i int)) {
	if n == 0 {
		return
	}
	sem := make(chan struct{}, max(limit, 1))
	var wg sync.WaitGroup
	for i := range n {
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			defer logging.Recover(log, fmt.Sprintf("parallel worker %d", i))
			fn(i)
		}()
	}
	wg.Wait()
}
