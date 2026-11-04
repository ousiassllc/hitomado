// Package exportfile は「保存先を利用者に選ばせ、そこへファイルを書く」処理の、OS に依存しない部分。
//
// デスクトップ（Windows）は保存ダイアログが返すパスへそのまま書く（PathTarget）。
// Android は保存ダイアログに当たる Storage Access Framework（ACTION_CREATE_DOCUMENT）が返すのがパスではなく
// content:// の Uri なので、Go はアプリの cache 配下の一時ファイルへ書き、Java が選ばれた Uri へストリームコピーする
// （Saver。Issue #85）。JNI（Java との呼び出し）は platform の *_android.go にあり、ここは build tag なしの純 Go で
// ホストでもテストできる。
package exportfile

import "context"

// Target は保存先。SaveFile が返し、呼び出し側は Path へ内容を書いて Commit し、最後に Close する。
type Target interface {
	// Path は呼び出し側が内容を書き込むローカルのパス。
	Path() string
	// Commit は書き込み済みの内容を利用者が選んだ保存先へ確定させ、利用者に見せる保存先（パスまたはファイル名）を返す。
	Commit(ctx context.Context) (location string, err error)
	// Close は確定されなかった一時ファイルと保存先の選択状態を片付ける。Commit の後に呼んでもよく、何度呼んでもよい。
	Close()
}

// PathTarget はダイアログで選ばれたパスへそのまま書く Target（デスクトップ）。
type PathTarget struct{ path string }

// NewPathTarget は path へ直接書く Target を返す。
func NewPathTarget(path string) *PathTarget { return &PathTarget{path: path} }

// Path は選ばれたパス。
func (t *PathTarget) Path() string { return t.path }

// Commit は何もしない（Path へ書いた時点で保存先に入っている）。
func (t *PathTarget) Commit(context.Context) (string, error) { return t.path, nil }

// Close は何もしない。
func (t *PathTarget) Close() {}
