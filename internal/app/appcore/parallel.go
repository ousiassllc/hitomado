package appcore

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/ousiassllc/hitomado/internal/logging"
)

// SearchParallelism は 1 つの操作内で同時に走らせる Gmail 呼び出し数の上限（レート制限はクライアント側）。
const SearchParallelism = 8

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
