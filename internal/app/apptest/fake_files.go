package apptest

import (
	"context"
	"sync"

	"github.com/ousiassllc/hitomado/internal/exportfile"
)

// FakeFiles は app.FileDialog のフェイク。Cancel=true で「キャンセル」、それ以外は Path を返す。
// Location が空でなければ Commit の結果（利用者に見せる保存先）をそれにする（Android のようにパスと違う場合）。
type FakeFiles struct {
	mu sync.Mutex

	Path      string
	Cancel    bool
	Err       error
	CommitErr error
	Location  string
	Suggested []string // SaveFile に渡された suggestedName
	Committed int      // Commit された回数
	Closed    int      // Close された回数
}

// SaveFile は設定どおりの結果を返す。
func (f *FakeFiles) SaveFile(_ context.Context, suggestedName string) (exportfile.Target, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Suggested = append(f.Suggested, suggestedName)
	if f.Err != nil {
		return nil, false, f.Err
	}
	if f.Cancel {
		return nil, false, nil
	}
	return &fakeTarget{f: f, PathTarget: exportfile.NewPathTarget(f.Path)}, true, nil
}

// fakeTarget は FakeFiles が返す Target。Commit・Close の呼ばれた回数を数える。
type fakeTarget struct {
	*exportfile.PathTarget
	f *FakeFiles
}

func (t *fakeTarget) Commit(ctx context.Context) (string, error) {
	t.f.mu.Lock()
	defer t.f.mu.Unlock()
	t.f.Committed++
	if t.f.CommitErr != nil {
		return "", t.f.CommitErr
	}
	if t.f.Location != "" {
		return t.f.Location, nil
	}
	return t.PathTarget.Commit(ctx)
}

func (t *fakeTarget) Close() {
	t.f.mu.Lock()
	defer t.f.mu.Unlock()
	t.f.Closed++
}
