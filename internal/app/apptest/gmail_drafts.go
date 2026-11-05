package apptest

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/ousiassllc/hitomado/internal/gmail"
)

// CreateDraft は Drafts／DraftRaw に保存し、"draft-N" を返す。
func (f *FakeGmail) CreateDraft(_ context.Context, accountID string, raw []byte, threadID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("CreateDraft", accountID); err != nil {
		return "", err
	}
	f.seq++
	id := fmt.Sprintf("draft-%d", f.seq)
	k := Key(accountID, id)
	f.storeDraft(accountID, k, id, threadID, raw)
	return id, nil
}

// UpdateDraft は MIME を置き換える。無ければ ErrNotFound。
func (f *FakeGmail) UpdateDraft(_ context.Context, accountID, draftID string, raw []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("UpdateDraft", accountID, draftID); err != nil {
		return err
	}
	k := Key(accountID, draftID)
	old, ok := f.Drafts[k]
	if !ok {
		return gmail.ErrNotFound
	}
	// 実 Gmail と同じく、更新で下書きのメッセージ ID（と添付の attachmentId）は新しくなる。
	f.dropDraftAttachments(accountID, old)
	f.storeDraft(accountID, k, draftID, old.ThreadID, raw)
	return nil
}

// GetDraft は Drafts を返す（CreateDraft／UpdateDraft に渡された MIME を解析した内容）。無ければ ErrNotFound。
func (f *FakeGmail) GetDraft(_ context.Context, accountID, draftID string) (gmail.DraftFull, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("GetDraft", accountID, draftID); err != nil {
		return gmail.DraftFull{}, err
	}
	d, ok := f.Drafts[Key(accountID, draftID)]
	if !ok {
		return gmail.DraftFull{}, gmail.ErrNotFound
	}
	return d, nil
}

// DeleteDraft は下書きを削除する。無ければ ErrNotFound。
func (f *FakeGmail) DeleteDraft(_ context.Context, accountID, draftID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("DeleteDraft", accountID, draftID); err != nil {
		return err
	}
	k := Key(accountID, draftID)
	old, ok := f.Drafts[k]
	if !ok {
		return gmail.ErrNotFound
	}
	f.dropDraftAttachments(accountID, old)
	delete(f.Drafts, k)
	delete(f.DraftRaw, k)
	return nil
}

// FindDraftForThread は Drafts から ThreadID が一致する下書き（ID 昇順で先頭）を返す。
func (f *FakeGmail) FindDraftForThread(_ context.Context, accountID, threadID string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("FindDraftForThread", accountID, threadID); err != nil {
		return "", false, err
	}
	var ids []string
	for k, d := range f.Drafts {
		if acc, _, _ := strings.Cut(k, "/"); acc == accountID && d.ThreadID == threadID {
			ids = append(ids, d.ID)
		}
	}
	if len(ids) == 0 {
		return "", false, nil
	}
	sort.Strings(ids)
	return ids[0], true, nil
}

// ListLabels は Labels を返す（未設定は空）。
func (f *FakeGmail) ListLabels(_ context.Context, accountID string) ([]gmail.Label, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("ListLabels", accountID); err != nil {
		return nil, err
	}
	return slices.Clone(f.Labels[accountID]), nil
}

// GetSignature は Signatures を返す（未設定は ""）。呼び出した sendAsEmail は SignatureRequests に残る。
func (f *FakeGmail) GetSignature(_ context.Context, accountID, sendAsEmail string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.enter("GetSignature", accountID); err != nil {
		return "", err
	}
	f.SignatureRequests = append(f.SignatureRequests, sendAsEmail)
	return f.Signatures[accountID], nil
}

// MetaOf は ThreadFull から threads.get(format=metadata) 相当を作る。
func MetaOf(tf gmail.ThreadFull) gmail.ThreadMeta {
	m := gmail.ThreadMeta{ID: tf.ID, HistoryID: tf.HistoryID, Messages: make([]gmail.MessageMeta, len(tf.Messages))}
	for i, msg := range tf.Messages {
		m.Messages[i] = msg.MessageMeta
	}
	return m
}

func hasAllLabels(tf gmail.ThreadFull, labels []string) bool {
	for _, l := range labels {
		if !slices.ContainsFunc(tf.Messages, func(m gmail.MessageFull) bool { return slices.Contains(m.LabelIDs, l) }) {
			return false
		}
	}
	return true
}

func matches(tf gmail.ThreadFull, needle string) bool {
	if needle == "" {
		return true
	}
	return slices.ContainsFunc(tf.Messages, func(m gmail.MessageFull) bool {
		return strings.Contains(strings.ToLower(m.Subject), needle) || strings.Contains(strings.ToLower(m.Snippet), needle)
	})
}

func lastSnippet(tf gmail.ThreadFull) string {
	if len(tf.Messages) == 0 {
		return ""
	}
	return tf.Messages[len(tf.Messages)-1].Snippet
}

func latestMillis(tf gmail.ThreadFull) int64 {
	var at int64
	for _, m := range tf.Messages {
		at = max(at, m.InternalDate.UnixMilli())
	}
	return at
}

func (f *FakeGmail) deriveStubs(accountID string, q gmail.ListQuery) []gmail.ThreadStub {
	type item struct {
		stub gmail.ThreadStub
		at   int64
	}
	var items []item
	needle := strings.ToLower(strings.TrimSpace(q.Q))
	for key, tf := range f.Threads {
		acc, _, _ := strings.Cut(key, "/")
		if acc != accountID || !hasAllLabels(tf, q.LabelIDs) || !matches(tf, needle) {
			continue
		}
		items = append(items, item{stub: gmail.ThreadStub{ID: tf.ID, HistoryID: tf.HistoryID, Snippet: lastSnippet(tf)}, at: latestMillis(tf)})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].at != items[j].at {
			return items[i].at > items[j].at
		}
		return items[i].stub.ID < items[j].stub.ID
	})
	stubs := make([]gmail.ThreadStub, len(items))
	for i, it := range items {
		stubs[i] = it.stub
	}
	return stubs
}
