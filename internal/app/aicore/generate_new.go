package aicore

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/domain/textconv"
)

// 新規作成の AI 下書きに渡す、入力済みの件名・宛先の上限（長いものは渡さずに切る）。
const (
	newDraftSubjectRunes = 500
	newDraftMaxRecipient = 20
)

// NewDraftAPI は新規作成の AI 下書き（#365）のバインディング面（AIService が埋め込んで公開する。実装は Service）。
type NewDraftAPI interface {
	// GenerateNewDraft は new セッションの本文を、AI への指示（必須）と入力済みの件名・宛先から生成する（F-08。#365）。保存しない。
	GenerateNewDraft(ctx context.Context, sessionID, requestID, instruction string, content dto.NewDraftContext) (dto.ReplyDraft, error)
}

var _ NewDraftAPI = (*Service)(nil)

// GenerateNewDraft は new セッションの本文を、利用者の指示から生成する（F-08。#365）。スレッドが無いので、
// AI への指示と、入力済みなら件名・宛先（To のみ）・送信元アカウントの表示名と署名から作る。署名は本文に含めない。
// 指示は必須（前後の空白を除いて空なら、通信せず VALIDATION）。そのほかの前提・中止・タイムアウト・上限は GenerateReplyDraft と同じで、
// 指示・件名・宛先は保存せず、ログ・エラー文面にも残さない。送信元が未選択のときは、表示名・署名なしで生成する。
func (s *Service) GenerateNewDraft(ctx context.Context, sessionID, requestID, instruction string, content dto.NewDraftContext) (dto.ReplyDraft, error) {
	if requestID == "" {
		return dto.ReplyDraft{}, apperr.New(apperr.Validation, "リクエスト ID が必要です。")
	}
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return dto.ReplyDraft{}, apperr.New(apperr.Validation, "AI への指示を入力してください。")
	}
	if utf8.RuneCountInString(instruction) > ai.MaxInstructionRunes {
		return dto.ReplyDraft{}, apperr.New(apperr.Validation, fmt.Sprintf("AI への指示は %d 文字までです。", ai.MaxInstructionRunes))
	}
	mode, accountID, err := s.sessions.SenderOf(sessionID)
	if err != nil {
		return dto.ReplyDraft{}, err
	}
	if mode != model.ModeNew {
		return dto.ReplyDraft{}, apperr.New(apperr.Validation, "この AI 下書きは新規作成でのみ使えます。")
	}
	runCtx, finish, err := s.begin(ctx, requestID)
	if err != nil {
		return dto.ReplyDraft{}, err
	}
	defer finish()
	cfg, key, err := s.precheck(runCtx)
	if err != nil {
		return dto.ReplyDraft{}, s.precheckError(runCtx, err)
	}

	in := ai.ReplyInput{Instruction: instruction, New: newMailContext(content)}
	if accountID != "" {
		acct, err := s.d.Store.GetAccount(runCtx, accountID)
		if err != nil {
			return dto.ReplyDraft{}, s.mapError(runCtx, err, accountID)
		}
		in.AccountDisplayName = acct.DisplayName
		in.SignatureText = strings.TrimSpace(textconv.HTMLToText(acct.SignatureHTML))
	}
	return s.generateDraft(runCtx, cfg, key, in)
}

// newMailContext は入力済みの件名・宛先を AI へ渡す形にする（CR/LF は空白に畳み、件名と宛先の数は上限で切る）。
func newMailContext(c dto.NewDraftContext) *ai.NewMailContext {
	subject := strings.Join(strings.Fields(c.Subject), " ")
	if r := []rune(subject); len(r) > newDraftSubjectRunes {
		subject = string(r[:newDraftSubjectRunes])
	}
	to := make([]string, 0, min(len(c.To), newDraftMaxRecipient))
	for _, a := range c.To {
		if len(to) == newDraftMaxRecipient {
			break
		}
		email := strings.TrimSpace(a.Email)
		if email == "" {
			continue
		}
		if name := strings.Join(strings.Fields(a.Name), " "); name != "" {
			to = append(to, fmt.Sprintf("%s <%s>", name, email))
		} else {
			to = append(to, email)
		}
	}
	return &ai.NewMailContext{Subject: subject, To: to}
}
