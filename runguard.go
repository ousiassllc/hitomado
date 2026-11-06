package main

import "sync/atomic"

// running は run が実行中か。Android では Activity が OS に作り直されると（ブラウザでの認可から戻ったときなど）、
// WailsBridge.nativeInit が再び呼ばれ、プロセスに残っている Go のランタイムで main が 2 回目に起動される。
// Wails の App.Run は 1 プロセスで 1 回しか呼べず、2 回目は "application is running or a previous run has failed" で失敗し、
// main が os.Exit(1) でプロセスごと終えて、実行中の認可（127.0.0.1 の待受）と同期を道連れにしてしまう。
// 実行中の 2 回目の main は何もせず戻し、最初の実行を続けさせる（#184・#185・#86）。
var running atomic.Bool

// runExclusive は run が実行中でなければ fn を実行し、その結果を返す（ran=true）。実行中なら fn を呼ばず ran=false を返す。
// fn が戻ったあとは再び実行できる（最初の実行が終わっている場合の 2 回目は、従来どおり Wails 側のエラーで終わる）。
func runExclusive(fn func() error) (ran bool, err error) {
	if !running.CompareAndSwap(false, true) {
		return false, nil
	}
	defer running.Store(false)
	return true, fn()
}
