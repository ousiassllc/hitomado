package inboxzerotests

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
)

// Jev へ渡すのは手がかり（パートの Content-Type と添付の種類・拡張子）だけ。端末内ではカレンダーと決めない。
// text/calendar や .ics があっても、AI が other と答えれば候補にならず、手がかりが無くても AI が calendar と答えれば候補になる。
func TestInboxZeroSendsInviteCluesWithoutDecidingLocally(t *testing.T) {
	f := newIZFixture(t)
	f.jevReady(t)
	f.setReasons(t, dto.InboxZeroReasons{Calendar: true})
	inv := msg("i1", "t-inv", 0, "INBOX")
	inv.PartTypes = []string{"text/plain", "text/html", "text/calendar; method=REQUEST", "application/ics"}
	inv.Attachments = []model.AttachmentInfo{{ID: "a1", Filename: "秘密の予定.ICS", MimeType: "application/ics", Size: 100}}
	bare := msg("b1", "t-bare", 0, "INBOX")
	bare.PartTypes = []string{"text/html"}
	refs := []dto.ThreadRef{f.thread(t, "t-inv", inv), f.thread(t, "t-bare", bare)}
	f.answers(map[string]ai.ClassifyAnswer{"t-bare": pick("calendar", 0.95)})

	res, err := f.find(refs...)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"t-bare": "calendar"}, reasonsOf(res), "手がかりでは決めず、AI の答えに従う")
	require.Equal(t, 2, f.Classifier.Calls())
	for _, in := range f.Classifier.Inputs {
		switch {
		case strings.Contains(in.State, "subject t-inv"):
			assert.Contains(t, in.State, "\nContent-Types: text/plain, text/html, text/calendar; method=REQUEST, application/ics\n")
			assert.Contains(t, in.State, "\nAttachments: application/ics (.ics)\n")
			assert.NotContains(t, in.State, "秘密の予定", "ファイル名は送らない")
		default:
			assert.Contains(t, in.State, "\nContent-Types: text/html\n")
			assert.NotContains(t, in.State, "Attachments:")
		}
	}
}

// カレンダーの招待・通知は端末内では判定せず、Inbox Zero の AI が「calendar」を高い確率で選んだものだけが候補になる（#377）。
// 確率の下限（0.8）に届かないもの・other は外れる。
func TestInboxZeroCalendarIsJudgedByAI(t *testing.T) {
	f := newIZFixture(t)
	f.jevReady(t)
	f.setReasons(t, dto.InboxZeroReasons{Calendar: true})
	refs := []dto.ThreadRef{f.mail(t, "t-cal"), f.mail(t, "t-edge"), f.mail(t, "t-low"), f.mail(t, "t-plain")}
	f.answers(map[string]ai.ClassifyAnswer{"t-cal": pick("calendar", 0.95), "t-edge": pick("calendar", 0.8), "t-low": pick("calendar", 0.79)})
	res, err := f.find(refs...)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"t-cal": "calendar", "t-edge": "calendar"}, reasonsOf(res))
	assert.Equal(t, 4, res.Examined)
	assert.Zero(t, res.Skipped)
	assert.Equal(t, 4, f.Classifier.Calls(), "カレンダーのメールも AI に送る")
}

// 判定の AI に渡す選択肢にカレンダーがあり、通知と重ならない説明になっている。位置は通知より前・other の前。
func TestInboxZeroCalendarCriterionIsSentToClassifier(t *testing.T) {
	f := newIZFixture(t)
	f.jevReady(t)
	f.setReasons(t, dto.InboxZeroReasons{Calendar: true})
	_, err := f.find(f.mail(t, "t-cal"))
	require.NoError(t, err)
	require.Equal(t, 1, f.Classifier.Calls())
	desc := map[string]string{}
	var names []string
	for _, c := range f.Classifier.Inputs[0].Criteria {
		desc[c.Name] = c.Description
		names = append(names, c.Name)
	}
	assert.Equal(t, []string{"ad", "calendar", "notification", "newsletter", "other"}, names)
	assert.Contains(t, desc["calendar"], "招待")
	assert.Contains(t, desc["notification"], "カレンダーの招待・通知は除く")
}

// カレンダーがオフなら、モデルが「calendar」を選んでも候補にしない（2 番目の候補にも落とさない）。オンなら候補になる。
func TestInboxZeroCalendarOffExcludesCalendarChoice(t *testing.T) {
	f := newIZFixture(t)
	f.jevReady(t)
	f.setReasons(t, dto.InboxZeroReasons{Ad: true, Notification: true, Newsletter: true, Replied: true})
	cal, note := f.mail(t, "t-cal"), f.mail(t, "t-note")
	f.answers(map[string]ai.ClassifyAnswer{
		"t-cal":  {Choice: "calendar", Probabilities: map[string]float64{"calendar": 0.9, "notification": 0.09, "other": 0.01}},
		"t-note": pick("notification", 0.99),
	})
	res, err := f.find(cal, note)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"t-note": "notification"}, reasonsOf(res))
	assert.Zero(t, res.Skipped)

	f.setReasons(t, allReasons)
	res, err = f.find(cal)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"t-cal": "calendar"}, reasonsOf(res))
}

// Google カレンダーの通知の送信元でも、端末内の近道は無く、ほかのメールと同じく AI の判定に従う。
func TestInboxZeroGoogleCalendarSenderGoesToClassifier(t *testing.T) {
	f := newIZFixture(t)
	f.jevReady(t)
	f.setReasons(t, dto.InboxZeroReasons{Calendar: true})
	m := msg("g1", "t-google", 0, "INBOX")
	m.From = model.Address{Name: "Google カレンダー", Email: "calendar-notification@google.com"}
	m.Subject = "招待状: 打ち合わせ"
	ref := f.thread(t, "t-google", m)

	res, err := f.find(ref)
	require.NoError(t, err)
	assert.Empty(t, res.Candidates, "AI が other と答えれば候補にしない")
	require.Equal(t, 1, f.Classifier.Calls(), "送信元だけでは判定せず AI に送る")
	assert.Contains(t, f.Classifier.Inputs[0].State, "From: Google カレンダー <calendar-notification@google.com>")

	f.Classifier.Fn = func(_ context.Context, _ ai.ClassifyInput) (ai.ClassifyAnswer, error) {
		return pick("calendar", 0.9), nil
	}
	res, err = f.find(ref)
	require.NoError(t, err)
	assert.Equal(t, []dto.InboxZeroCandidate{{Ref: ref, From: "Google カレンダー", Subject: "招待状: 打ち合わせ", Reason: "calendar"}}, res.Candidates)
}

// 優先順位は 返信済み（端末内）→ AI。最後が自分の送信なら、AI がカレンダーと答えうるメールでも「返信済み」で、AI に送らない。
func TestInboxZeroRepliedBeatsCalendar(t *testing.T) {
	f := newIZFixture(t)
	f.jevReady(t)
	mine := msg("m-mine", "t-mine", 0, "SENT")
	mine.From = model.Address{Email: f.a.Email}
	f.answers(map[string]ai.ClassifyAnswer{"t-mine": pick("calendar", 0.99)})
	res, err := f.find(f.thread(t, "t-mine", mine))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"t-mine": "replied"}, reasonsOf(res))
	assert.Zero(t, f.Classifier.Calls())
}

// カレンダーだけをオンにした設定は AI の前提（Inbox Zero に選んだ AI かキー・同意）が要る。返信済みだけなら要らない。
func TestInboxZeroCalendarOnlyNeedsAI(t *testing.T) {
	f := newIZFixture(t)
	f.setReasons(t, dto.InboxZeroReasons{Calendar: true})
	_, err := f.find(f.mail(t, "t-cal"))
	requireCode(t, "AI_NOT_CONFIGURED", err)

	f.setReasons(t, dto.InboxZeroReasons{Replied: true, Calendar: true})
	_, err = f.find(f.mail(t, "t-cal"))
	requireCode(t, "AI_NOT_CONFIGURED", err)
	assert.Zero(t, f.Classifier.Created()+f.Classifier.Calls(), "前提を満たすまで通信しない")

	f.setReasons(t, dto.InboxZeroReasons{Replied: true})
	_, err = f.find(f.mail(t, "t-cal"))
	require.NoError(t, err)
	assert.Zero(t, f.Classifier.Created()+f.Classifier.Calls(), "返信済みだけなら AI を使わない")
}

// 確認一覧の形（差出人・件名・理由）は変わらない。カレンダーの候補も、最新の（下書きでない）メッセージの差出人と件名を出す。
func TestInboxZeroCalendarCandidateShape(t *testing.T) {
	f := newIZFixture(t)
	f.jevReady(t)
	f.setReasons(t, dto.InboxZeroReasons{Calendar: true})
	m := msg("c1", "t-cal", 0, "INBOX")
	m.Subject = "招待状: 打ち合わせ"
	m.From = model.Address{Name: "Google カレンダー", Email: "calendar-notification@google.com"}
	draft := msg("d1", "t-cal", 5, "DRAFT")
	ref := f.thread(t, "t-cal", m, draft)
	f.Classifier.Fn = func(_ context.Context, _ ai.ClassifyInput) (ai.ClassifyAnswer, error) {
		return pick("calendar", 0.9), nil
	}
	res, err := f.find(ref)
	require.NoError(t, err)
	assert.Equal(t, []dto.InboxZeroCandidate{{Ref: ref, From: "Google カレンダー", Subject: "招待状: 打ち合わせ", Reason: "calendar"}}, res.Candidates)
}
