// Package gmail は Gmail API クライアントの抽象と MIME 構築を提供する。
package gmail

import (
	"context"
	"errors"
	"time"

	"github.com/ousiassllc/hitomado/internal/domain/model"
)

// Gmail API 呼び出しのエラー分類。実装は errors.Is で判定できる形で wrap して返す。
// internal/app/apperr がこれらをバインディングのエラーコードへ写像する。
var (
	ErrNeedsReauth    = errors.New("gmail: needs reauth")           // invalid_grant（トークン失効・取り消し）
	ErrClientRejected = errors.New("gmail: oauth client rejected")  // invalid_client／unauthorized_client（Client ID／Secret が拒否された）
	ErrOffline        = errors.New("gmail: offline")                // ネットワーク不通
	ErrRateLimited    = errors.New("gmail: rate limited")           // 429（バックオフで解消せず）
	ErrNotFound       = errors.New("gmail: not found")              // 404（スレッド・下書き等）
	ErrHistoryExpired = errors.New("gmail: history id expired")     // history.list の 404
	ErrAttachmentGone = errors.New("gmail: attachment unavailable") // 添付が取得できない
	ErrGeneric        = errors.New("gmail: api error")              // 上記以外の Gmail API エラー（4xx／5xx）
	ErrSendUncertain  = errors.New("gmail: send result unknown")    // Send の応答を受け取れず、送信済みかも確認できない（再送すると二重送信になり得る）
)

// 次の 2 つは Google が 403 で返した reason から判定する補助の sentinel。*APIError に対して errors.Is で調べる（単独の公開エラーとしては返さない）。
var (
	ErrAPIDisabled       = errors.New("gmail: api disabled")       // Gmail API が Google Cloud のプロジェクトで無効（SERVICE_DISABLED／accessNotConfigured）。分類は ErrGeneric
	ErrScopeInsufficient = errors.New("gmail: scope insufficient") // 権限（スコープ）不足（ACCESS_TOKEN_SCOPE_INSUFFICIENT／insufficientPermissions）。分類は ErrNeedsReauth
)

// Profile は users.getProfile の結果。
type Profile struct {
	Email     string // 小文字
	HistoryID string // 現在の historyId
}

// ListQuery は threads.list の条件。
type ListQuery struct {
	Q          string   // Gmail 検索クエリ（そのまま渡す）
	LabelIDs   []string // 例: ["INBOX"]
	PageToken  string
	MaxResults int64
}

// ThreadStub は threads.list の 1 件。
type ThreadStub struct {
	ID        string
	HistoryID string
	Snippet   string
}

// ThreadList は threads.list の結果。
type ThreadList struct {
	Threads       []ThreadStub
	NextPageToken string // 空なら末尾
}

// MessageMeta は format=metadata で得られるメッセージ情報。
type MessageMeta struct {
	ID            string
	ThreadID      string
	InternalDate  time.Time
	LabelIDs      []string
	Snippet       string
	From          model.Address
	To            []model.Address
	Cc            []model.Address
	Subject       string
	HasAttachment bool // multipart/mixed 等からの近似
}

// ThreadMeta は threads.get(format=metadata)。Messages は時系列昇順。
type ThreadMeta struct {
	ID        string
	HistoryID string
	Messages  []MessageMeta
}

// InlinePart は本文中の cid 参照で使うインライン画像。
type InlinePart struct {
	MimeType string
	Data     []byte
}

// MessageFull は format=full のメッセージ。
type MessageFull struct {
	MessageMeta
	MessageID    string // Message-ID ヘッダ
	References   []string
	ReplyTo      []model.Address
	BodyHTML     string
	BodyText     string
	Attachments  []model.AttachmentInfo
	InlineImages map[string]InlinePart // key は mimeparse.NormalizeCID 済みの Content-ID（山括弧・cid: なし、小文字、%デコード済み）
}

// ThreadFull は threads.get(format=full)。Messages は時系列昇順（下書きも含む。呼び出し側が DRAFT を除く）。
type ThreadFull struct {
	ID        string
	HistoryID string
	Messages  []MessageFull
}

// HistoryAdded は history の messagesAdded 1 件。
type HistoryAdded struct {
	MessageID string
	ThreadID  string
	LabelIDs  []string
}

// HistoryPage は history.list の全ページを統合した結果。
type HistoryPage struct {
	LatestHistoryID  string   // 次回の startHistoryId
	ChangedThreadIDs []string // 追加・削除・ラベル変更のあったスレッド（重複なし）
	Added            []HistoryAdded
}

// SentMessage は messages.send の結果。
type SentMessage struct {
	MessageID string
	ThreadID  string
}

// DraftFull は drafts.get の結果。
type DraftFull struct {
	ID          string
	ThreadID    string
	Message     MessageFull
	Bcc         []model.Address // Bcc ヘッダ（下書き復元用。MessageFull には持たない）
	Attachments []model.AttachmentInfo
}

// DraftSummary は下書き一覧の 1 件（drafts.get format=metadata）。
type DraftSummary struct {
	ID        string
	ThreadID  string
	UpdatedAt time.Time // 下書きのメッセージの internalDate（保存のたびに新しくなる）
	To        []model.Address
	Subject   string
	Snippet   string // 本文の先頭（Gmail の snippet。HTML のエスケープを戻したプレーンテキスト）
	IsReply   bool   // In-Reply-To がある（返信の下書き。新規の下書きは無い）
}

// DraftList は ListDrafts の結果。Truncated は limit を超える下書きがあって切り捨てたこと。
type DraftList struct {
	Items     []DraftSummary
	Truncated bool
}

// Label は users.labels の 1 件。
type Label struct {
	ID      string
	Name    string
	Type    string // system / user
	ColorBg string
	ColorFg string
}

// Client は Gmail API のアカウント別クライアント。実装はアカウントごとに TokenSource と
// rate.Limiter を持ち、429／5xx を指数バックオフ（初回 1 秒・上限 60 秒・ジッタ付き）で再試行する。
type Client interface {
	Profile(ctx context.Context, accountID string) (Profile, error)
	// ListHistory は全ページを取得して統合する。startHistoryId が失効していれば ErrHistoryExpired。
	ListHistory(ctx context.Context, accountID, startHistoryID string) (HistoryPage, error)
	ListThreads(ctx context.Context, accountID string, q ListQuery) (ThreadList, error)
	GetThreadMetadata(ctx context.Context, accountID, threadID string) (ThreadMeta, error) // 無ければ ErrNotFound
	GetThreadFull(ctx context.Context, accountID, threadID string) (ThreadFull, error)
	ModifyThread(ctx context.Context, accountID, threadID string, add, remove []string) error
	TrashThread(ctx context.Context, accountID, threadID string) error
	UntrashThread(ctx context.Context, accountID, threadID string) error
	GetAttachment(ctx context.Context, accountID, messageID, attachmentID string) ([]byte, error)
	Send(ctx context.Context, accountID string, raw []byte, threadID string) (SentMessage, error)
	CreateDraft(ctx context.Context, accountID string, raw []byte, threadID string) (draftID string, err error)
	UpdateDraft(ctx context.Context, accountID, draftID string, raw []byte) error
	GetDraft(ctx context.Context, accountID, draftID string) (DraftFull, error)
	DeleteDraft(ctx context.Context, accountID, draftID string) error
	FindDraftForThread(ctx context.Context, accountID, threadID string) (draftID string, ok bool, err error)
	// ListDrafts は下書きの一覧（drafts.list と各下書きの drafts.get format=metadata）を返す。先頭から limit 件まで（Gmail が返す順）で、
	// 超える分は切り捨てて Truncated を立てる。一覧の取得中に削除された下書きは含めない。本文・件名・宛先は利用者の情報なので、ログにもエラー文言にも出さない。
	ListDrafts(ctx context.Context, accountID string, limit int) (DraftList, error)
	ListLabels(ctx context.Context, accountID string) ([]Label, error)
	// GetSignature は users.settings.sendAs.get で sendAsEmail の署名（HTML。未設定は ""）を返す。
	// 署名の本文は利用者の情報なので、ログにもエラー文言にも出さない。
	GetSignature(ctx context.Context, accountID, sendAsEmail string) (string, error)
}

// AccountLookup は Client が OAuth 資格情報を解決するための参照（store アダプタが実装する）。
// リフレッシュトークンは secrets.RefreshTokenKey(accountID)、Client Secret は
// secrets.OAuthClientSecretKey(oauthClientRowID) から secrets.Store 経由で読む。
type AccountLookup interface {
	OAuthCredentials(ctx context.Context, accountID string) (clientID, oauthClientRowID string, err error)
}
