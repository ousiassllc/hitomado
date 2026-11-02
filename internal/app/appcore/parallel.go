package appcore

import "sync"

// SearchParallelism は 1 つの操作内で同時に走らせる Gmail 呼び出し数の上限（レート制限はクライアント側）。
const SearchParallelism = 8

// ParallelDo は fn(0..n-1) を最大 limit 並列で実行し、すべての完了を待つ。
func ParallelDo(n, limit int, fn func(i int)) {
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
			fn(i)
		}()
	}
	wg.Wait()
}
