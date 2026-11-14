package inboxzerotests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
)

// calendarMail は受信箱の 1 通のスレッドで、iCalendar のパートを持つもの（Google カレンダーなどの招待）。
func (f *izFixture) calendarMail(t *testing.T, tid string) dto.ThreadRef {
	m := msg(tid, tid, 0, "INBOX")
	m.HasCalendar = true
	return f.thread(t, tid, m)
}

// カレンダーの招待・通知（iCalendar のパートを持つ、または Google カレンダーからの送信）は「カレンダー」の理由で候補になる。AI は使わない。
// 最後の 1 通（下書きを除く）で見る。
func TestInboxZeroCalendarCandidates(t *testing.T) {
	f := newIZFixture(t)
	f.setReasons(t, dto.InboxZeroReasons{Calendar: true})
	google := msg("g1", "t-google", 0, "INBOX")
	google.From = model.Address{Name: "Google カレンダー", Email: "Calendar-Notification@google.com"}
	earlier := msg("e1", "t-earlier", 0, "INBOX")
	earlier.HasCalendar = true
	draft := msg("d1", "t-draft", 5, "DRAFT")
	draft.HasCalendar = true
	refs := []dto.ThreadRef{
		f.calendarMail(t, "t-ics"),
		f.thread(t, "t-google", google),
		f.mail(t, "t-plain"),
		f.thread(t, "t-earlier", earlier, msg("e2", "t-earlier", 1, "INBOX")), // 招待のあとに、普通の返事が来た
		f.thread(t, "t-draft", msg("d0", "t-draft", 0, "INBOX"), draft),       // 下書きだけが iCalendar を持つ
	}
	res, err := f.find(refs...)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"t-ics": "calendar", "t-google": "calendar"}, reasonsOf(res))
	assert.Equal(t, 5, res.Examined)
	assert.Zero(t, f.Classifier.Created()+f.Classifier.Calls(), "カレンダーの判定は端末内（AI を使わない）")
}

// カレンダーがオフなら、カレンダーの招待は候補に入らない。Jev が「通知」と判定しうるメールでも、AI に送らず、広告・通知など別の理由に寄せない。
func TestInboxZeroCalendarOffExcludesAndIsNotSentToAI(t *testing.T) {
	f := newIZFixture(t)
	f.jevReady(t)
	f.setReasons(t, dto.InboxZeroReasons{Ad: true, Notification: true, Newsletter: true, Replied: true})
	cal, plain := f.calendarMail(t, "t-cal"), f.mail(t, "t-note")
	f.answers(map[string]ai.ClassifyAnswer{"t-cal": pick("notification", 0.99), "t-note": pick("notification", 0.99)})
	res, err := f.find(cal, plain)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"t-note": "notification"}, reasonsOf(res))
	require.Equal(t, 1, f.Classifier.Calls(), "カレンダーのメールは AI にも送らない")
	assert.Contains(t, f.Classifier.Inputs[0].State, "subject t-note")

	f.setReasons(t, allReasons)
	res, err = f.find(cal)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"t-cal": "calendar"}, reasonsOf(res), "オンなら、Jev より先に「カレンダー」で分類する")
	assert.Equal(t, 1, f.Classifier.Calls())
}

// 優先順位は 返信済み → カレンダー → AI。最後が自分の送信なら、カレンダーの返信（iCalendar つき）も「返信済み」。
func TestInboxZeroReasonPrecedence(t *testing.T) {
	f := newIZFixture(t)
	f.jevReady(t)
	mine := msg("m-mine", "t-mine", 0, "SENT")
	mine.From = model.Address{Email: f.a.Email}
	mine.HasCalendar = true
	res, err := f.find(f.thread(t, "t-mine", mine))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"t-mine": "replied"}, reasonsOf(res))
	assert.Zero(t, f.Classifier.Calls())
}

// カレンダーだけ（または返信済みとカレンダーだけ）をオンにしたときは、AI のキー・選択・同意が何も無くても動き、AI の工場も作らない。
func TestInboxZeroCalendarOnlyNeedsNoAI(t *testing.T) {
	f := newIZFixture(t)
	f.setReasons(t, dto.InboxZeroReasons{Calendar: true})
	res, err := f.find(f.calendarMail(t, "t-cal"), f.mail(t, "t-plain"))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"t-cal": "calendar"}, reasonsOf(res))

	f.setReasons(t, dto.InboxZeroReasons{Replied: true, Calendar: true})
	_, err = f.find(f.calendarMail(t, "t-cal"))
	require.NoError(t, err)
	assert.Zero(t, f.Classifier.Created()+f.Classifier.Calls())

	f.setReasons(t, dto.InboxZeroReasons{Ad: true, Calendar: true})
	_, err = f.find(f.calendarMail(t, "t-cal"))
	requireCode(t, "AI_NOT_CONFIGURED", err) // AI の理由が 1 つでもオンなら AI の前提が要る
}

// 確認一覧の形（差出人・件名・理由）は変わらない。カレンダーの候補も、最新の（下書きでない）メッセージの差出人と件名を出す。
func TestInboxZeroCalendarCandidateShape(t *testing.T) {
	f := newIZFixture(t)
	f.setReasons(t, dto.InboxZeroReasons{Calendar: true})
	m := msg("c1", "t-cal", 0, "INBOX")
	m.HasCalendar, m.Subject = true, "招待状: 打ち合わせ"
	m.From = model.Address{Name: "Google カレンダー", Email: "calendar-notification@google.com"}
	ref := f.thread(t, "t-cal", m)
	res, err := f.find(ref)
	require.NoError(t, err)
	assert.Equal(t, []dto.InboxZeroCandidate{{Ref: ref, From: "Google カレンダー", Subject: "招待状: 打ち合わせ", Reason: "calendar"}}, res.Candidates)
}
