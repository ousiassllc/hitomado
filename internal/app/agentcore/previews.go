package agentcore

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/domain/signature"
	"github.com/ousiassllc/hitomado/internal/domain/textconv"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

// 下書きのプレビュー（本文の最初の数行。プレーンテキスト）は DB に保存せず、メモリだけで持つ（NF-S08）。
const (
	// previewCacheSize はメモリに持つプレビューの上限（項目数）。超えたら古いものから捨てる。
	previewCacheSize = 100
	// previewFetchConcurrency は ListItems が Gmail.GetDraft を同時に呼ぶ上限。
	previewFetchConcurrency = 4
	// previewFetchTimeout は GetDraft 1 回の上限、previewListTimeout は ListItems 1 回で下書きの取得に使う合計の上限。
	// 超えた分のプレビューは空で返す（一覧は失敗させない）。
	previewFetchTimeout = 3 * time.Second
	previewListTimeout  = 5 * time.Second
)

// DraftReader は Gmail の下書きの読み取り口（gmail.Client の GetDraft だけ。送信・作成・更新・削除は持たない）。
type DraftReader interface {
	GetDraft(ctx context.Context, accountID, draftID string) (gmail.DraftFull, error)
}

type previewEntry struct{ draftID, text string }

// previewCache は項目 ID をキーにした、下書きのプレビューのメモリキャッシュ（上限つき。古いものから捨てる）。
// エントリは作ったときの DraftID を持ち、項目の DraftID が変わった（下書きを作り直した）ら使わない。
type previewCache struct {
	mu      sync.Mutex
	entries map[string]previewEntry
	order   []string // 挿入順（先頭が最古）
}

func newPreviewCache() *previewCache { return &previewCache{entries: map[string]previewEntry{}} }

func (c *previewCache) get(itemID, draftID string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[itemID]
	if !ok || e.draftID != draftID {
		return "", false
	}
	return e.text, true
}

func (c *previewCache) put(itemID, draftID, text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[itemID]; !ok {
		c.order = append(c.order, itemID)
		if len(c.order) > previewCacheSize {
			delete(c.entries, c.order[0])
			c.order = c.order[1:]
		}
	}
	c.entries[itemID] = previewEntry{draftID: draftID, text: text}
}

// hasDraftPreview は項目がプレビューを出す対象か（ready／later／done で下書きがあるもの）。
func hasDraftPreview(it store.AgentItem) bool {
	return it.DraftID != "" && (it.Status == store.AgentStatusReady || it.Status == store.AgentStatusLater || it.Status == store.AgentStatusDone)
}

// previewsFor は items のうちプレビューを出す対象の項目 ID → プレビューを返す。メモリに無いものは Gmail.GetDraft で作る
// （並行度は previewFetchConcurrency。合計は previewListTimeout まで）。取得に失敗・超過したものは空で返し、エラーにはしない。
func (s *Service) previewsFor(ctx context.Context, items []store.AgentItem) map[string]string {
	out := map[string]string{}
	var missing []store.AgentItem
	for _, it := range items {
		if !hasDraftPreview(it) {
			continue
		}
		if text, ok := s.previews.get(it.ID, it.DraftID); ok {
			out[it.ID] = text
		} else {
			missing = append(missing, it)
		}
	}
	if len(missing) == 0 || s.d.Gmail == nil {
		return out
	}

	ctx, cancel := context.WithTimeout(ctx, previewListTimeout)
	defer cancel()
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, previewFetchConcurrency)
	)
	for _, it := range missing {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break // 合計の締め切りを過ぎた分は、プレビュー空のまま
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			text, ok := s.fetchPreview(ctx, it)
			if !ok {
				return
			}
			s.previews.put(it.ID, it.DraftID, text)
			mu.Lock()
			out[it.ID] = text
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

// fetchPreview は Gmail の下書きから冒頭のプレビューを作る。失敗はアカウント ID とエラーの種類だけをログに残して false。
func (s *Service) fetchPreview(ctx context.Context, it store.AgentItem) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, previewFetchTimeout)
	defer cancel()
	d, err := s.d.Gmail.GetDraft(ctx, it.AccountID, it.DraftID)
	if err != nil {
		s.log.Warn("agent: draft preview failed", "account", it.AccountID, "kind", errKind(apperr.FromInfra(err, it.AccountID)))
		return "", false
	}
	return preview(draftLeadText(d)), true
}

// draftLeadText は下書きの本文のうち、署名・引用より前（AI が書いた部分）をプレーンテキストにする。
func draftLeadText(d gmail.DraftFull) string {
	html := d.Message.BodyHTML
	if html == "" {
		return d.Message.BodyText
	}
	for _, marker := range []string{signature.Marker, signature.QuoteMarker} {
		if at := strings.Index(html, marker); at >= 0 {
			html = html[:max(strings.LastIndex(html[:at], "<"), 0)] // マーカーを持つ開始タグの手前まで
		}
	}
	return textconv.HTMLToText(html)
}
