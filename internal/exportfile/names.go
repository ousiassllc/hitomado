package exportfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/ousiassllc/hitomado/internal/app/appcore"
)

// octetStream は種類が分からないときの MIME。
const octetStream = "application/octet-stream"

// knownTypes は Go の標準の対応表（OS の mime.types・Windows のレジストリを読む。Android には無い）に頼らず、
// 拡張子から MIME への対応を固定する。保存・表示（Android）と、メールの添付の送信（全 OS。composecore）が同じ表を使う。
var knownTypes = map[string]string{
	".pdf": "application/pdf", ".zip": "application/zip", ".txt": "text/plain", ".log": "text/plain",
	".csv": "text/csv", ".json": "application/json", ".html": "text/html", ".htm": "text/html",
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp",
	".doc": "application/msword", ".xls": "application/vnd.ms-excel", ".ppt": "application/vnd.ms-powerpoint",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".mp3":  "audio/mpeg", ".mp4": "video/mp4", ".ics": "text/calendar", ".eml": "message/rfc822",
	".md": "text/markdown", ".xml": "application/xml", ".rtf": "application/rtf", ".svg": "image/svg+xml",
	".css": "text/css", ".js": "text/javascript", ".mjs": "text/javascript", ".wasm": "application/wasm", ".avif": "image/avif",
	".heic": "image/heic", ".gz": "application/gzip", ".tar": "application/x-tar", ".7z": "application/x-7z-compressed",
	".wav": "audio/wav", ".mov": "video/quicktime",
}

// MIMEForName はファイル名の拡張子から MIME を推定する。分からなければ application/octet-stream。
// 結果は knownTypes だけで決まり、OS の mime.types・レジストリには依存しない（OS・端末で Content-Type が変わらない）。
func MIMEForName(name string) string {
	if t, ok := knownTypes[strings.ToLower(filepath.Ext(name))]; ok {
		return t
	}
	return octetStream
}

// DisplayName は保存先に渡すファイル名を安全にする。メールの添付のファイル名は信頼できない入力なので、
// 区切り（/ \）・制御文字・予約名・長さを appcore.SanitizeFilename と同じ規則で直す。
// ディレクトリ部分（../ など）が付いていても末尾のファイル名だけを使う。
func DisplayName(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	return appcore.SanitizeFilename(name)
}

// 開く操作（OpenPath）のエラー。
var (
	// ErrFolderNotSupported はフォルダを開こうとした（Android にはアプリ専用の領域を開くファイルマネージャーの経路が無い）。
	ErrFolderNotSupported = errors.New("opening a folder is not supported")
	// ErrNoApp はそのファイルを開けるアプリが端末に無い。
	ErrNoApp = errors.New("no app can open this file")
)

// OpenRequest は「ファイルを既定のアプリで開く」前の検査をして、渡す MIME を返す。
// 存在しなければエラー、フォルダなら ErrFolderNotSupported。パスが開いてよい場所の中かは Java 側（FileProvider の公開範囲）が検査する。
func OpenRequest(path string) (mimeType string, err error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if fi.IsDir() {
		return "", ErrFolderNotSupported
	}
	return MIMEForName(path), nil
}
