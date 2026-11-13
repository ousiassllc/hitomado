// Package apptest は internal/app のテスト用の共有フェイクと Deps の組み立てを提供する。
//
// 外部 I/O（Gmail・OAuth・ファイルダイアログ・ブラウザ・通知・更新確認）はフェイクで、
// ストアは実 SQLite（t.TempDir() 内の一時ファイル）、秘密情報は secrets.NewMemory()、時計は clock.Fake。
// このパッケージは internal/app を import するため、テストは外部テストパッケージ（package app_test）に置くこと。
package apptest

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/ousiassllc/hitomado/internal/gmail"
)

// Key はフェイクのマップキー（アカウント ID とスレッド ID 等を "/" で連結する）。
func Key(parts ...string) string { return strings.Join(parts, "/") }

// SentRecord は FakeGmail.Send の呼び出し記録。
type SentRecord struct {
	AccountID string
	Raw       []byte
	ThreadID  string
}

// ModifyRecord は ModifyThread の呼び出し記録。
type ModifyRecord struct {
	AccountID string
	ThreadID  string
	Add       []string
	Remove    []string
}

// FakeGmail はスクリプト可能なインメモリの gmail.Client。並行利用できる。
//
// 公開フィールドはテストの準備と結果の確認に使う（呼び出し中に直接書き換えない）。
// Err はメソッド名で失敗を注入する: キーは "Method"（全体）・"Method:accountID"・"Method:accountID:threadID"
// （GetDraft などスレッド以外が対象のメソッドは第 3 要素に draft／attachment／message ID）。
type FakeGmail struct {
	mu sync.Mutex

	Profiles    map[string]gmail.Profile      // key: accountID
	Threads     map[string]gmail.ThreadFull   // key: Key(accountID, threadID)
	Lists       map[string][]gmail.ThreadStub // ListThreads の結果を固定する。key: Key(accountID, q)
	Attachments map[string][]byte             // key: Key(accountID, messageID, attachmentID)
	Drafts      map[string]gmail.DraftFull    // key: Key(accountID, draftID)
	DraftRaw    map[string][]byte             // CreateDraft／UpdateDraft に渡された MIME。key は Drafts と同じ
	Labels      map[string][]gmail.Label      // key: accountID
	History     map[string]gmail.HistoryPage  // key: accountID
	Signatures  map[string]string             // GetSignature の結果（HTML）。key: accountID

	Sent      []SentRecord
	Modified  []ModifyRecord
	Trashed   []string // Key(accountID, threadID)
	Untrashed []string // Key(accountID, threadID)
	Forgotten []string // Forget(accountID) の呼び出し
	// SignatureRequests は GetSignature に渡された sendAsEmail の呼び出し順。
	SignatureRequests []string
	Calls             []string // "Method:accountID" の呼び出し順

	Err map[string]error
	// Panic は Err と同じキーで、呼び出しを panic させる（別 goroutine の異常を再現する）。値が panic の値。
	Panic map[string]any

	seq int
}

var _ gmail.Client = (*FakeGmail)(nil)

// NewFakeGmail は空の FakeGmail を返す。
func NewFakeGmail() *FakeGmail {
	return &FakeGmail{
		Profiles:    map[string]gmail.Profile{},
		Threads:     map[string]gmail.ThreadFull{},
		Lists:       map[string][]gmail.ThreadStub{},
		Attachments: map[string][]byte{},
		Drafts:      map[string]gmail.DraftFull{},
		DraftRaw:    map[string][]byte{},
		Labels:      map[string][]gmail.Label{},
		History:     map[string]gmail.HistoryPage{},
		Signatures:  map[string]string{},
		Err:         map[string]error{},
		Panic:       map[string]any{},
	}
}

// enter は呼び出しを記録し、注入されたエラーを返す（f.mu を保持して呼ぶ）。
func (f *FakeGmail) enter(method, accountID string, extra ...string) error {
	f.Calls = append(f.Calls, method+":"+accountID)
	keys := []string{method, method + ":" + accountID}
	if len(extra) > 0 {
		keys = append(keys, method+":"+accountID+":"+extra[0])
	}
	for i := len(keys) - 1; i >= 0; i-- {
		if v, ok := f.Panic[keys[i]]; ok {
			panic(v)
		}
		if err := f.Err[keys[i]]; err != nil {
			return err
		}
	}
	return nil
}

// CallCount は method の呼び出し回数を返す（全アカウント合計）。
func (f *FakeGmail) CallCount(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.Calls {
		if strings.HasPrefix(c, method+":") {
			n++
		}
	}
	return n
}

// Forget は gmail.Client の実装が持つ任意メソッド（再接続時にトークンキャッシュを捨てる）の代わり。
func (f *FakeGmail) Forget(accountID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Forgotten = append(f.Forgotten, accountID)
}

// Profile は Profiles を返す。無ければ ErrNotFound。
func (f *FakeGmail) Profile(_ context.Context, accountID string) (gmail.Profile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("Profile", accountID); err != nil {
		return gmail.Profile{}, err
	}
	p, ok := f.Profiles[accountID]
	if !ok {
		return gmail.Profile{}, gmail.ErrNotFound
	}
	return p, nil
}

// ListHistory は History を返す（未設定は空）。
func (f *FakeGmail) ListHistory(_ context.Context, accountID, _ string) (gmail.HistoryPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("ListHistory", accountID); err != nil {
		return gmail.HistoryPage{}, err
	}
	return f.History[accountID], nil
}

// ListThreads は Lists（Key(accountID, q)）があればそれを、無ければ Threads から導出して返す。
// 導出: 最新メッセージ日時の降順・ID 昇順。LabelIDs はすべてを含むスレッド、Q は件名・スニペットの部分一致（空は全件）。
// PageToken は 10 進のオフセット、MaxResults 未指定は 100。
func (f *FakeGmail) ListThreads(_ context.Context, accountID string, q gmail.ListQuery) (gmail.ThreadList, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("ListThreads", accountID); err != nil {
		return gmail.ThreadList{}, err
	}
	stubs, ok := f.Lists[Key(accountID, q.Q)]
	if !ok {
		stubs = f.deriveStubs(accountID, q)
	}
	offset := 0
	if q.PageToken != "" {
		n, err := strconv.Atoi(q.PageToken)
		if err != nil || n < 0 {
			return gmail.ThreadList{}, fmt.Errorf("apptest: bad page token %q: %w", q.PageToken, gmail.ErrGeneric)
		}
		offset = n
	}
	size := int(q.MaxResults)
	if size <= 0 {
		size = 100
	}
	if offset > len(stubs) {
		offset = len(stubs)
	}
	end := min(offset+size, len(stubs))
	out := gmail.ThreadList{Threads: slices.Clone(stubs[offset:end])}
	if end < len(stubs) {
		out.NextPageToken = strconv.Itoa(end)
	}
	return out, nil
}

// GetThreadMetadata は Threads から導出する。無ければ ErrNotFound。
func (f *FakeGmail) GetThreadMetadata(_ context.Context, accountID, threadID string) (gmail.ThreadMeta, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("GetThreadMetadata", accountID, threadID); err != nil {
		return gmail.ThreadMeta{}, err
	}
	tf, ok := f.Threads[Key(accountID, threadID)]
	if !ok {
		return gmail.ThreadMeta{}, gmail.ErrNotFound
	}
	return MetaOf(tf), nil
}

// GetThreadFull は Threads を返す。無ければ ErrNotFound。
// 実 Gmail と同じく、スレッドに紐づく下書き（Drafts のメッセージが DRAFT ラベル付きのもの）も含める。
func (f *FakeGmail) GetThreadFull(_ context.Context, accountID, threadID string) (gmail.ThreadFull, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("GetThreadFull", accountID, threadID); err != nil {
		return gmail.ThreadFull{}, err
	}
	tf, ok := f.Threads[Key(accountID, threadID)]
	if !ok {
		return gmail.ThreadFull{}, gmail.ErrNotFound
	}
	tf.Messages = slices.Clone(tf.Messages)
	for k, d := range f.Drafts {
		if strings.HasPrefix(k, Key(accountID, "")) && d.ThreadID == threadID && slices.Contains(d.Message.LabelIDs, "DRAFT") &&
			!slices.ContainsFunc(tf.Messages, func(m gmail.MessageFull) bool { return m.ID == d.Message.ID }) {
			tf.Messages = append(tf.Messages, d.Message)
		}
	}
	return tf, nil
}

// ModifyThread は全メッセージのラベルを更新して記録する。
func (f *FakeGmail) ModifyThread(_ context.Context, accountID, threadID string, add, remove []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("ModifyThread", accountID, threadID); err != nil {
		return err
	}
	if err := f.relabel(accountID, threadID, add, remove); err != nil {
		return err
	}
	f.Modified = append(f.Modified, ModifyRecord{AccountID: accountID, ThreadID: threadID, Add: slices.Clone(add), Remove: slices.Clone(remove)})
	return nil
}

// TrashThread は TRASH を付与し INBOX を除去して Trashed に記録する。
func (f *FakeGmail) TrashThread(_ context.Context, accountID, threadID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("TrashThread", accountID, threadID); err != nil {
		return err
	}
	if err := f.relabel(accountID, threadID, []string{"TRASH"}, []string{"INBOX"}); err != nil {
		return err
	}
	f.Trashed = append(f.Trashed, Key(accountID, threadID))
	return nil
}

// UntrashThread は TRASH を除去して Untrashed に記録する（INBOX は復元しない）。
func (f *FakeGmail) UntrashThread(_ context.Context, accountID, threadID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("UntrashThread", accountID, threadID); err != nil {
		return err
	}
	if err := f.relabel(accountID, threadID, nil, []string{"TRASH"}); err != nil {
		return err
	}
	f.Untrashed = append(f.Untrashed, Key(accountID, threadID))
	return nil
}

func (f *FakeGmail) relabel(accountID, threadID string, add, remove []string) error {
	k := Key(accountID, threadID)
	tf, ok := f.Threads[k]
	if !ok {
		return gmail.ErrNotFound
	}
	msgs := slices.Clone(tf.Messages)
	for i := range msgs {
		labels := slices.DeleteFunc(slices.Clone(msgs[i].LabelIDs), func(l string) bool { return slices.Contains(remove, l) })
		for _, l := range add {
			if !slices.Contains(labels, l) {
				labels = append(labels, l)
			}
		}
		msgs[i].LabelIDs = labels
	}
	tf.Messages = msgs
	f.Threads[k] = tf
	return nil
}

// GetAttachment は Attachments を返す。無ければ ErrAttachmentGone。
func (f *FakeGmail) GetAttachment(_ context.Context, accountID, messageID, attachmentID string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("GetAttachment", accountID, attachmentID); err != nil {
		return nil, err
	}
	data, ok := f.Attachments[Key(accountID, messageID, attachmentID)]
	if !ok {
		return nil, gmail.ErrAttachmentGone
	}
	return slices.Clone(data), nil
}

// Send は Sent に記録し、擬似 ID を返す。threadID が空なら新しいスレッド ID を振る。
func (f *FakeGmail) Send(_ context.Context, accountID string, raw []byte, threadID string) (gmail.SentMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("Send", accountID); err != nil {
		return gmail.SentMessage{}, err
	}
	f.seq++
	f.Sent = append(f.Sent, SentRecord{AccountID: accountID, Raw: slices.Clone(raw), ThreadID: threadID})
	if threadID == "" {
		threadID = fmt.Sprintf("sent-thread-%d", f.seq)
	}
	return gmail.SentMessage{MessageID: fmt.Sprintf("sent-%d", f.seq), ThreadID: threadID}, nil
}
