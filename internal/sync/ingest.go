package syncer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

// fetched は 1 スレッドのメタデータ取得結果。gone は Gmail 側に存在しない（または空）。
type fetched struct {
	id   string
	meta gmail.ThreadMeta
	gone bool
}

// fetchThreads は ids のメタデータを最大 fetchConcurrency 並列で取得する。結果は ids の順。
// ErrNotFound は gone として扱い、それ以外の最初のエラーで全体を中止して返す。
func (s *Scheduler) fetchThreads(parent context.Context, accountID string, ids []string) ([]fetched, error) {
	out := make([]fetched, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	var (
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
	)
	sem := make(chan struct{}, fetchConcurrency)
loop:
	for i, id := range ids {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			break loop
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			meta, err := s.gmail.GetThreadMetadata(ctx, accountID, id)
			switch {
			case err == nil:
				out[i] = fetched{id: id, meta: meta, gone: len(meta.Messages) == 0}
			case errors.Is(err, gmail.ErrNotFound):
				out[i] = fetched{id: id, gone: true}
			default:
				once.Do(func() {
					firstErr = err
					cancel()
				})
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := parent.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// toSnapshot はメタデータを DB 反映用のスナップショットへ変換する。
func toSnapshot(accountID string, m gmail.ThreadMeta) store.ThreadSnapshot {
	snap := store.ThreadSnapshot{
		AccountID: accountID,
		ThreadID:  m.ID,
		HistoryID: m.HistoryID,
		Messages:  make([]store.MessageSnapshot, 0, len(m.Messages)),
	}
	for i, msg := range m.Messages {
		if i == 0 {
			snap.Subject = msg.Subject
		}
		snap.HasAttachment = snap.HasAttachment || msg.HasAttachment
		snap.Messages = append(snap.Messages, store.MessageSnapshot{
			ID:           msg.ID,
			InternalDate: msg.InternalDate,
			FromName:     msg.From.Name,
			FromEmail:    strings.ToLower(msg.From.Email),
			Snippet:      msg.Snippet,
			LabelIDs:     msg.LabelIDs,
		})
	}
	return snap
}

// snapshots は gone でない結果をスナップショットにする（順序維持）。
func snapshots(accountID string, fs []fetched) []store.ThreadSnapshot {
	out := make([]store.ThreadSnapshot, 0, len(fs))
	for _, f := range fs {
		if !f.gone {
			out = append(out, toSnapshot(accountID, f.meta))
		}
	}
	return out
}

// ingest は取得結果を DB に反映する。存在しないスレッドは削除する。
// 新規メッセージと連絡先は cyc に集める。
func (s *Scheduler) ingest(ctx context.Context, cyc *cycle, fs []fetched) error {
	accountID := cyc.acct.ID
	if err := s.ensureLabels(ctx, accountID, snapshots(accountID, fs)); err != nil {
		return err
	}
	for _, f := range fs {
		if f.gone {
			if err := s.store.DeleteThread(ctx, accountID, f.id); err != nil && !errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("syncer: delete thread: %w", err)
			}
			cyc.markChanged(f.id)
			continue
		}
		res, err := s.store.ApplyThread(ctx, toSnapshot(accountID, f.meta))
		if err != nil {
			return fmt.Errorf("syncer: apply thread: %w", err)
		}
		cyc.markChanged(f.id)
		cyc.recordNew(f.meta, res.NewMessageIDs)
	}
	return nil
}

// recordNew は新規メッセージ ID と、その差出人・宛先・Cc を集める。
func (c *cycle) recordNew(m gmail.ThreadMeta, newIDs []string) {
	if len(newIDs) == 0 {
		return
	}
	set := make(map[string]struct{}, len(newIDs))
	for _, id := range newIDs {
		set[id] = struct{}{}
	}
	c.newMsgs[m.ID] = set
	for _, msg := range m.Messages {
		if _, ok := set[msg.ID]; !ok {
			continue
		}
		c.contacts = appendAddr(c.contacts, msg.From)
		c.contacts = appendAddrs(c.contacts, msg.To)
		c.contacts = appendAddrs(c.contacts, msg.Cc)
	}
}

func appendAddr(dst []model.Address, a model.Address) []model.Address {
	if a.Email == "" {
		return dst
	}
	a.Email = strings.ToLower(a.Email)
	return append(dst, a)
}

func appendAddrs(dst []model.Address, as []model.Address) []model.Address {
	for _, a := range as {
		dst = appendAddr(dst, a)
	}
	return dst
}

// ensureLabels は snaps に現れるラベル ID が DB に無ければ、ラベル一覧を取り直す。
func (s *Scheduler) ensureLabels(ctx context.Context, accountID string, snaps []store.ThreadSnapshot) error {
	if len(snaps) == 0 {
		return nil
	}
	known, err := s.store.ListLabels(ctx, accountID)
	if err != nil {
		return fmt.Errorf("syncer: list labels: %w", err)
	}
	have := make(map[string]struct{}, len(known))
	for _, l := range known {
		have[l.ID] = struct{}{}
	}
	for _, snap := range snaps {
		for _, m := range snap.Messages {
			for _, id := range m.LabelIDs {
				if _, ok := have[id]; !ok {
					return s.refreshLabels(ctx, accountID)
				}
			}
		}
	}
	return nil
}

// refreshLabels は Gmail のラベル一覧で DB を置き換える。
func (s *Scheduler) refreshLabels(ctx context.Context, accountID string) error {
	ls, err := s.gmail.ListLabels(ctx, accountID)
	if err != nil {
		return err
	}
	labels := make([]store.Label, 0, len(ls))
	for _, l := range ls {
		labels = append(labels, store.Label{ID: l.ID, Name: l.Name, Type: l.Type, ColorBg: l.ColorBg, ColorFg: l.ColorFg})
	}
	if err := s.store.ReplaceLabels(ctx, accountID, labels); err != nil {
		return fmt.Errorf("syncer: replace labels: %w", err)
	}
	return nil
}
