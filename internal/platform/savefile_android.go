//go:build android

package platform

import (
	"context"
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/ousiassllc/hitomado/internal/exportfile"
)

// SaveFile は保存先を利用者に選ばせる（Android の Storage Access Framework。ACTION_CREATE_DOCUMENT）。
// Wails の Dialog.SaveFile は Android では使えない。選択が済むと、内容の書き込み先の一時ファイル（アプリの cache 配下）を持つ
// Target を返し、Commit で Java が選ばれた保存先へストリームコピーする（巨大な byte[] を JNI に通さない）。
// キャンセルは ok=false。流れと結果の待ち方は internal/exportfile と build/android の DocumentExporter。
func (f *Files) SaveFile(ctx context.Context, suggestedName string) (exportfile.Target, bool, error) {
	t, ok, err := f.saver.Pick(ctx, suggestedName)
	if err != nil {
		return nil, false, fmt.Errorf("save dialog: %w", err)
	}
	return t, ok, nil
}

// newSaver は Java の DocumentExporter を使う Saver を作り、Java からの結果（hitomado:document イベント）を渡す。
func newSaver(app *application.App) *exportfile.Saver {
	s := exportfile.NewSaver(javaDocuments{})
	app.Event.On(exportfile.EventName, func(e *application.CustomEvent) {
		if r, ok := exportfile.ParseResult(e.Data); ok {
			s.Deliver(r)
		}
	})
	return s
}

// javaDocuments は exportfile.Native の実装。WailsBridge.document*（Java）を呼ぶ。
type javaDocuments struct{}

func (javaDocuments) TempDir() (string, error) { return callJava("documentTempDir", struct{}{}) }

func (javaDocuments) Begin(id int, name, mime string) error {
	_, err := callJava("documentBegin", map[string]any{"id": id, "name": name, "mime": mime})
	return err
}

func (javaDocuments) Commit(id int, path string) error {
	_, err := callJava("documentCommit", map[string]any{"id": id, "path": path})
	return err
}

func (javaDocuments) Abort(id int) error {
	_, err := callJava("documentAbort", map[string]any{"id": id})
	return err
}
