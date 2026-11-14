package aicore

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/logging"
)

// Inbox Zero（#314・#377）の定数。
const (
	// inboxZeroMaxRefs は 1 回に調べるスレッド数の上限（重複を除く）。
	inboxZeroMaxRefs = 100
	// inboxZeroConcurrency は AI への同時リクエスト数の上限。
	inboxZeroConcurrency = 4
	// inboxZeroBodyRunes は AI へ送る本文の長さ（先頭からの文字数）。
	inboxZeroBodyRunes = 500
	// inboxZeroThreshold は候補にする確率の下限。選ばれた選択肢の確率（probabilities[choice]）に当てる。
	// Jev は確率を返し、チャット系のモデルは自己申告の確信度（0〜1）を返す。どちらも同じ下限で、確信が足りなければ候補にしない。
	inboxZeroThreshold = 0.8
)

// Gmail のシステムラベル。
const (
	labelSent   = "SENT"
	labelUnread = "UNREAD"
)

// classifyCriteria は判定の AI へ渡す選択肢（この順。Jev でもチャット系のモデルでも同じ）。無効にした理由も含めて常に全部を渡す
// （無効な理由のメールを「その他」へ寄せず、その理由として選ばせたうえで候補から外すため）。
// カレンダーは AI に聞かず端末内で先に判定するので、選択肢に含めない。
var classifyCriteria = []ai.Criterion{
	{Name: dto.InboxZeroReasonAd, Description: "商品・サービス・イベントなどを宣伝・販売促進する広告のメール（セール、キャンペーン、クーポン、新商品の案内など）。"},
	{Name: dto.InboxZeroReasonNotification, Description: "サービスやシステムが自動で送る通知だけのメール（ログイン通知、配送状況、利用明細、SNS の更新通知など）で、返信も対応も求めないもの。"},
	{Name: dto.InboxZeroReasonNewsletter, Description: "定期的に配信されるニュースレター・メールマガジン・記事やブログの更新まとめ。"},
	{Name: choiceOther, Description: "通常のメール・個人宛て・重要なもの・判断できない。"},
}

// choiceOther は「どれにも当てはまらない」選択肢の名前。候補にならない。
const choiceOther = "other"

const classifyInstructions = "このメールの種類として、最も当てはまるものを 1 つ選んでください。"

// inboxZeroRun は FindInboxZeroCandidates 1 回分の実行の状態。
type inboxZeroRun struct {
	s       *Service
	reasons dto.InboxZeroReasons
	// aiReasons は AI で判定する理由（広告・通知・ニュースレター）が 1 つでもオンか。
	aiReasons bool
	// classifier は aiReasons が真のときだけ作る。provider はその AI（エラーの文面の分類に使う）。
	classifier ai.Classifier
	provider   string
	// emails はアカウント ID → そのアカウントのメールアドレス（replied がオンのときだけ）。
	emails map[string]string

	// abort は他のスレッドの処理を止める（致命的なエラーのとき）。
	abort context.CancelFunc
	mu    sync.Mutex
	fatal error
}

// slot は 1 スレッドの結果。1 スレッド 1 つの ID で書かれるので、完了後は排他なしで読める。
type slot struct {
	cand    *dto.InboxZeroCandidate
	skipped bool
}

// FindInboxZeroCandidates は refs（フロントが読み込み済みの一覧）のうち、設定でオンの理由に当てはまる「不要なメール」の候補を返す。
// 返信済み・カレンダーは端末内だけで判定し、広告・通知・ニュースレターは Inbox Zero に選んだ AI・モデルで判定する（確率・確信度が高いものだけ）。
// 前提（選んだ AI のキー・ログインと同意）は通信とスレッドの読み込みの前に確かめる。AI の理由がすべてオフなら、AI の前提は要らない。
// 中止（CancelGenerate・ctx）は CANCELLED。AI の 401／403・429・利用上限は全体を止め、それ以外の失敗は該当スレッドを skipped にする。
func (s *Service) FindInboxZeroCandidates(ctx context.Context, refs []dto.ThreadRef, requestID string) (dto.InboxZeroResult, error) {
	if err := validateRequestID(requestID); err != nil {
		return dto.InboxZeroResult{}, err
	}
	refs = dedupeRefs(refs)
	if len(refs) > inboxZeroMaxRefs {
		return dto.InboxZeroResult{}, apperr.New(apperr.Validation, fmt.Sprintf("一度に調べられるスレッドは %d 件までです。", inboxZeroMaxRefs))
	}
	runCtx, finish, err := s.begin(ctx, requestID)
	if err != nil {
		return dto.InboxZeroResult{}, err
	}
	defer finish()
	reasons, err := s.loadInboxZeroReasons(runCtx)
	if err != nil {
		return dto.InboxZeroResult{}, s.precheckError(runCtx, err)
	}
	if !reasons.Ad && !reasons.Notification && !reasons.Newsletter && !reasons.Replied && !reasons.Calendar {
		return dto.InboxZeroResult{}, apperr.New(apperr.Validation, "不要なメールの理由を 1 つ以上オンにしてください。")
	}
	run := &inboxZeroRun{s: s, reasons: reasons, aiReasons: reasons.Ad || reasons.Notification || reasons.Newsletter}
	if run.aiReasons {
		if err := run.setupClassifier(runCtx); err != nil {
			return dto.InboxZeroResult{}, s.precheckError(runCtx, err)
		}
	}

	if reasons.Replied {
		if run.emails, err = s.accountEmails(runCtx, refs); err != nil {
			return dto.InboxZeroResult{}, s.mapError(runCtx, err, "")
		}
	}
	return run.execute(runCtx, refs)
}

// dedupeRefs は refs から重複（同じアカウントの同じスレッド）を除く。順序は保つ。
func dedupeRefs(refs []dto.ThreadRef) []dto.ThreadRef {
	seen := make(map[dto.ThreadRef]struct{}, len(refs))
	out := make([]dto.ThreadRef, 0, len(refs))
	for _, r := range refs {
		if _, dup := seen[r]; dup {
			continue
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	return out
}

// setupClassifier は Inbox Zero に選んだ AI の前提（つながっている・同意済み）を確かめ（通信の前）、Classifier を作る。
func (r *inboxZeroRun) setupClassifier(ctx context.Context) error {
	sel, key, err := r.s.precheckInboxZero(ctx)
	if err != nil {
		return err
	}
	cl, err := r.s.d.Classifier(sel.Provider, sel.Model, sel.BaseURL, key)
	if err != nil {
		return initError(err)
	}
	r.classifier, r.provider = cl, sel.Provider
	return nil
}

// accountEmails は refs のアカウントのメールアドレスを返す（返信済みの判定用）。
func (s *Service) accountEmails(ctx context.Context, refs []dto.ThreadRef) (map[string]string, error) {
	emails := map[string]string{}
	for _, ref := range refs {
		if _, ok := emails[ref.AccountID]; ok {
			continue
		}
		acct, err := s.d.Store.GetAccount(ctx, ref.AccountID)
		if err != nil {
			return nil, err
		}
		emails[ref.AccountID] = acct.Email
	}
	return emails, nil
}

// execute は refs を最大 inboxZeroConcurrency 件ずつ並行に判定し、候補を渡された順に返す。
func (r *inboxZeroRun) execute(runCtx context.Context, refs []dto.ThreadRef) (dto.InboxZeroResult, error) {
	workCtx, abort := context.WithCancel(runCtx)
	defer abort()
	r.abort = abort

	slots := make([]slot, len(refs))
	sem := make(chan struct{}, inboxZeroConcurrency)
	var wg sync.WaitGroup
	for i, ref := range refs {
		sem <- struct{}{}
		if workCtx.Err() != nil {
			<-sem
			break
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			slots[i] = r.judgeSafe(workCtx, ref)
		}()
	}
	wg.Wait()

	if r.fatal != nil {
		return dto.InboxZeroResult{}, r.fatal
	}
	if runCtx.Err() != nil {
		return dto.InboxZeroResult{}, r.s.mapError(runCtx, context.Cause(runCtx), "")
	}
	res := dto.InboxZeroResult{Candidates: []dto.InboxZeroCandidate{}, Examined: len(refs)}
	for _, sl := range slots {
		switch {
		case sl.cand != nil:
			res.Candidates = append(res.Candidates, *sl.cand)
		case sl.skipped:
			res.Skipped++
		}
	}
	return res, nil
}

// fail は致命的なエラー（最初の 1 件だけ残す）を記録し、ほかのスレッドの処理を止める。
func (r *inboxZeroRun) fail(err error) {
	r.mu.Lock()
	if r.fatal == nil {
		r.fatal = err
	}
	r.mu.Unlock()
	r.abort()
}

// judgeSafe は judge を呼ぶ。別 goroutine で動くため panic は呼び出し元の recover に届かずプロセスを落とす。
// panic は記録し、そのスレッドだけ skipped にする（TypeSafe AI の失敗と同じ扱い）。
func (r *inboxZeroRun) judgeSafe(ctx context.Context, ref dto.ThreadRef) (sl slot) {
	var err error
	defer func() {
		if err != nil {
			sl = slot{skipped: true}
		}
	}()
	defer logging.RecoverTo(r.s.d.Logger, "inbox zero judge", &err)
	return r.judge(ctx, ref)
}

// judge は 1 スレッドを判定する。ctx が終わっていたら何も記録しない（結果は使われない）。
// 理由の優先順位は、返信済み（オンのとき）→ カレンダー → AI。カレンダーの招待・通知は、カレンダーがオフなら候補にせず
// （AI にも聞かず、広告・通知など別の理由に寄せない）、オンなら「カレンダー」を理由にする。判定はどれもスレッドの最後の 1 通（下書きを除く）を見る。
func (r *inboxZeroRun) judge(ctx context.Context, ref dto.ThreadRef) slot {
	thread, err := r.s.d.Threads.Get(ctx, ref.AccountID, ref.ThreadID)
	if err != nil {
		switch {
		case ctx.Err() != nil:
		case errors.Is(err, gmail.ErrNotFound):
			return slot{skipped: true} // 読み込んだあとに消えたスレッド
		default:
			r.fail(r.s.mapError(ctx, err, ref.AccountID))
		}
		return slot{}
	}
	msgs := nonDraftMessages(thread)
	if len(msgs) == 0 {
		return slot{}
	}
	latest := msgs[len(msgs)-1]
	cand := dto.InboxZeroCandidate{Ref: ref, From: senderLabel(latest), Subject: latest.Subject}
	if r.reasons.Replied && isOwnMessage(latest, r.emails[ref.AccountID]) {
		cand.Reason = dto.InboxZeroReasonReplied
		return slot{cand: &cand}
	}
	if isCalendarMail(latest) {
		if !r.reasons.Calendar {
			return slot{}
		}
		cand.Reason = dto.InboxZeroReasonCalendar
		return slot{cand: &cand}
	}
	reasons := r.aiEligible(msgs)
	if len(reasons) == 0 {
		return slot{}
	}
	reason, ok, err := r.classify(ctx, latest, reasons)
	switch {
	case err != nil && ctx.Err() == nil:
		return r.classifyFailure(err)
	case err != nil, !ok:
		return slot{}
	}
	cand.Reason = reason
	return slot{cand: &cand}
}

// classifyFailure は AI の失敗を扱う。認証（401／403）・レート制限（429）・ChatGPT の利用上限・対象外は、
// どのスレッドでも同じ結果になるので全体を止め、それ以外は該当スレッドだけ skipped にする。
func (r *inboxZeroRun) classifyFailure(err error) slot {
	if errors.Is(err, ai.ErrUnauthorized) || errors.Is(err, ai.ErrRateLimited) || errors.Is(err, ai.ErrUsageLimit) || errors.Is(err, ai.ErrNotEligible) {
		r.fail(r.s.mapProviderError(r.provider, "", err))
		return slot{}
	}
	return slot{skipped: true}
}

// aiEligible は AI の判定で候補にできる理由（オンのもの。ニュースレターはスレッドが未読のときだけ）。
func (r *inboxZeroRun) aiEligible(msgs []gmail.MessageFull) []string {
	if !r.aiReasons {
		return nil
	}
	var reasons []string
	if r.reasons.Ad {
		reasons = append(reasons, dto.InboxZeroReasonAd)
	}
	if r.reasons.Notification {
		reasons = append(reasons, dto.InboxZeroReasonNotification)
	}
	if r.reasons.Newsletter && slices.ContainsFunc(msgs, func(m gmail.MessageFull) bool { return slices.Contains(m.LabelIDs, labelUnread) }) {
		reasons = append(reasons, dto.InboxZeroReasonNewsletter)
	}
	return reasons
}

// classify は latest を AI に判定させる。eligible のどれかを高い確率で選んだときだけ、その理由を ok=true で返す。
// 選ばれた選択肢が対象外（オフの理由・other・未読でないニュースレター）なら、2 番目の候補には落とさず ok=false。
func (r *inboxZeroRun) classify(ctx context.Context, latest gmail.MessageFull, eligible []string) (reason string, ok bool, err error) {
	callCtx, stop := context.WithTimeout(ctx, r.s.timeout)
	defer stop()
	ans, err := r.classifier.Classify(callCtx, ai.ClassifyInput{State: classifyState(latest), Instructions: classifyInstructions, Criteria: classifyCriteria})
	if err != nil {
		return "", false, err
	}
	if !slices.Contains(eligible, ans.Choice) || ans.Probabilities[ans.Choice] < inboxZeroThreshold {
		return "", false, nil
	}
	return ans.Choice, true, nil
}
