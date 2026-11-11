package aicore

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/gmail"
)

// Inbox Zero（#314）の定数。
const (
	// inboxZeroMaxRefs は 1 回に調べるスレッド数の上限（重複を除く）。
	inboxZeroMaxRefs = 100
	// inboxZeroConcurrency は TypeSafe AI への同時リクエスト数の上限。
	inboxZeroConcurrency = 4
	// inboxZeroBodyRunes は TypeSafe AI へ送る本文の長さ（先頭からの文字数）。
	inboxZeroBodyRunes = 500
	// inboxZeroThreshold は候補にする確率の下限。選ばれた選択肢の確率（probabilities[choice]）に当てる（confidence ではない）。
	inboxZeroThreshold = 0.8
)

// Gmail のシステムラベル。
const (
	labelSent   = "SENT"
	labelUnread = "UNREAD"
)

// jevCriteria は TypeSafe AI へ渡す選択肢（この順）。無効にした理由も含めて常に全部を渡す
// （無効な理由のメールを「その他」へ寄せず、その理由として選ばせたうえで候補から外すため）。
var jevCriteria = []ai.Criterion{
	{Name: dto.InboxZeroReasonAd, Description: "商品・サービス・イベントなどを宣伝・販売促進する広告のメール（セール、キャンペーン、クーポン、新商品の案内など）。"},
	{Name: dto.InboxZeroReasonNotification, Description: "サービスやシステムが自動で送る通知だけのメール（ログイン通知、配送状況、利用明細、SNS の更新通知など）で、返信も対応も求めないもの。"},
	{Name: dto.InboxZeroReasonNewsletter, Description: "定期的に配信されるニュースレター・メールマガジン・記事やブログの更新まとめ。"},
	{Name: jevOther, Description: "通常のメール・個人宛て・重要なもの・判断できない。"},
}

// jevOther は「どれにも当てはまらない」選択肢の名前。候補にならない。
const jevOther = "other"

const jevInstructions = "このメールの種類として、最も当てはまるものを 1 つ選んでください。"

// inboxZeroRun は FindInboxZeroCandidates 1 回分の実行の状態。
type inboxZeroRun struct {
	s       *Service
	reasons dto.InboxZeroReasons
	// jevReasons は TypeSafe AI で判定する理由が 1 つでもオンか。
	jevReasons bool
	// classifier は jevReasons が真のときだけ作る。
	classifier ai.Classifier
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
// 返信済みは端末内だけで判定し、広告・通知・ニュースレターは TypeSafe AI（Jev）で判定する（確率が高いものだけ）。
// 前提（キー・同意）は通信とスレッドの読み込みの前に確かめる。TypeSafe AI の理由がすべてオフなら、キーも同意も要らない。
// 中止（CancelGenerate・ctx）は CANCELLED。TypeSafe AI の 401／403・429 は全体を止め、それ以外の失敗は該当スレッドを skipped にする。
func (s *Service) FindInboxZeroCandidates(ctx context.Context, refs []dto.ThreadRef, requestID string) (dto.InboxZeroResult, error) {
	if err := validateRequestID(requestID); err != nil {
		return dto.InboxZeroResult{}, err
	}
	refs = dedupeRefs(refs)
	if len(refs) > inboxZeroMaxRefs {
		return dto.InboxZeroResult{}, apperr.New(apperr.Validation, fmt.Sprintf("一度に調べられるスレッドは %d 件までです。", inboxZeroMaxRefs))
	}
	reasons, err := s.loadInboxZeroReasons(ctx)
	if err != nil {
		return dto.InboxZeroResult{}, err
	}
	if !reasons.Ad && !reasons.Notification && !reasons.Newsletter && !reasons.Replied {
		return dto.InboxZeroResult{}, apperr.New(apperr.Validation, "不要なメールの理由を 1 つ以上オンにしてください。")
	}
	run := &inboxZeroRun{s: s, reasons: reasons, jevReasons: reasons.Ad || reasons.Notification || reasons.Newsletter}
	if run.jevReasons {
		if err := run.setupClassifier(ctx); err != nil {
			return dto.InboxZeroResult{}, err
		}
	}

	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	if err := s.register(requestID, cancel); err != nil {
		return dto.InboxZeroResult{}, err
	}
	defer s.unregister(requestID)

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

// setupClassifier は TypeSafe AI のキーと同意を確かめ（通信の前）、Classifier を作る。
func (r *inboxZeroRun) setupClassifier(ctx context.Context) error {
	key, ok, err := r.s.apiKey(providerTypeSafe)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.New(apperr.AINotConfigured, "TypeSafe AI のキーを設定画面で登録してください。")
	}
	consented, err := r.s.hasConsent(ctx, providerTypeSafe, "")
	if err != nil {
		return err
	}
	if !consented {
		return apperr.New(apperr.AIConsentRequired, "メールの件名・差出人・本文の先頭が TypeSafe AI へ送信されることへの同意が必要です。")
	}
	r.classifier = r.s.d.Classifier(key)
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
			slots[i] = r.judge(workCtx, ref)
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

// judge は 1 スレッドを判定する。ctx が終わっていたら何も記録しない（結果は使われない）。
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
	if r.reasons.Replied && hasOwnMessage(msgs, r.emails[ref.AccountID]) {
		cand.Reason = dto.InboxZeroReasonReplied
		return slot{cand: &cand}
	}
	reasons := r.jevEligible(msgs)
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

// classifyFailure は TypeSafe AI の失敗を扱う。認証（401／403）・レート制限（429）は全体を止め、それ以外は該当スレッドだけ skipped にする。
func (r *inboxZeroRun) classifyFailure(err error) slot {
	if errors.Is(err, ai.ErrUnauthorized) || errors.Is(err, ai.ErrRateLimited) {
		r.fail(r.s.mapProviderError(providerTypeSafe, "", err))
		return slot{}
	}
	return slot{skipped: true}
}

// jevEligible は TypeSafe AI の判定で候補にできる理由（オンのもの。ニュースレターはスレッドが未読のときだけ）。
func (r *inboxZeroRun) jevEligible(msgs []gmail.MessageFull) []string {
	if !r.jevReasons {
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

// classify は latest を TypeSafe AI に判定させる。eligible のどれかを高い確率で選んだときだけ、その理由を ok=true で返す。
// 選ばれた選択肢が対象外（オフの理由・other・未読でないニュースレター）なら、2 番目の候補には落とさず ok=false。
func (r *inboxZeroRun) classify(ctx context.Context, latest gmail.MessageFull, eligible []string) (reason string, ok bool, err error) {
	callCtx, stop := context.WithTimeout(ctx, r.s.timeout)
	defer stop()
	ans, err := r.classifier.Classify(callCtx, ai.ClassifyInput{State: jevState(latest), Instructions: jevInstructions, Criteria: jevCriteria})
	if err != nil {
		return "", false, err
	}
	if !slices.Contains(eligible, ans.Choice) || ans.Probabilities[ans.Choice] < inboxZeroThreshold {
		return "", false, nil
	}
	return ans.Choice, true, nil
}

// jevState は TypeSafe AI へ送る内容（差出人・件名・本文の先頭 500 文字だけ）。
func jevState(m gmail.MessageFull) string {
	return "From: " + sender(m) + "\nSubject: " + m.Subject + "\n\n" + firstRunes(messageText(m), inboxZeroBodyRunes)
}

// sender は差出人を「表示名 <アドレス>」（表示名が無ければアドレス）にする。
func sender(m gmail.MessageFull) string {
	if m.From.Name == "" {
		return m.From.Email
	}
	return m.From.Name + " <" + m.From.Email + ">"
}

// firstRunes は s の先頭 n 文字（rune）。
func firstRunes(s string, n int) string {
	for i := range s {
		if n == 0 {
			return s[:i]
		}
		n--
	}
	return s
}

// nonDraftMessages はスレッドの下書きでないメッセージ（古い順）。
func nonDraftMessages(thread gmail.ThreadFull) []gmail.MessageFull {
	return slices.DeleteFunc(slices.Clone(thread.Messages), func(m gmail.MessageFull) bool { return slices.Contains(m.LabelIDs, labelDraft) })
}

// hasOwnMessage はメッセージに自分が送ったもの（SENT ラベル、または差出人が自分のアドレス。大文字小文字を区別しない）があるか。
func hasOwnMessage(msgs []gmail.MessageFull, email string) bool {
	return slices.ContainsFunc(msgs, func(m gmail.MessageFull) bool {
		return slices.Contains(m.LabelIDs, labelSent) || (email != "" && strings.EqualFold(strings.TrimSpace(m.From.Email), email))
	})
}
