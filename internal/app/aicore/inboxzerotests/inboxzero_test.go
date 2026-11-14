package aicoretests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

var allReasons = dto.InboxZeroReasons{Ad: true, Notification: true, Newsletter: true, Replied: true}

// izFixture は Inbox Zero のテスト環境（jevReady で TypeSafe AI のキーと同意を済ませる）。
type izFixture struct{ *aiFixture }

func newIZFixture(t *testing.T) *izFixture { return &izFixture{newAIFixture(t)} }

func (f *izFixture) jevReady(t *testing.T) {
	t.Helper()
	_, err := f.ai.SetAIApiKey(t.Context(), "typesafe", testAPIKey)
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(t.Context(), "typesafe", "")
	require.NoError(t, err)
}

func (f *izFixture) setReasons(t *testing.T, r dto.InboxZeroReasons) {
	t.Helper()
	_, err := f.ai.SetInboxZeroReasons(t.Context(), r)
	require.NoError(t, err)
}

// msg は Epoch の minute 分後のメッセージ。
func msg(id, tid string, minute int, labels ...string) gmail.MessageFull {
	return apptest.Msg(id, tid, apptest.Epoch.Add(time.Duration(minute)*time.Minute), labels...)
}

// mail は tid の受信箱の 1 通のスレッド（件名 "subject <tid>"、差出人 Sender）を置く。
func (f *izFixture) mail(t *testing.T, tid string, labels ...string) dto.ThreadRef {
	return f.thread(t, tid, msg(tid, tid, 0, append([]string{"INBOX"}, labels...)...))
}

func (f *izFixture) thread(t *testing.T, tid string, msgs ...gmail.MessageFull) dto.ThreadRef {
	t.Helper()
	f.PutThread(t, f.a.ID, tid, msgs...)
	return dto.ThreadRef{AccountID: f.a.ID, ThreadID: tid}
}

// answers は件名に tid を含むスレッドへの Jev の答えを決める（無ければ other 1.0）。
func (f *izFixture) answers(m map[string]ai.ClassifyAnswer) {
	f.Classifier.Fn = func(_ context.Context, in ai.ClassifyInput) (ai.ClassifyAnswer, error) {
		for tid, a := range m {
			if strings.Contains(in.State, "Subject: subject "+tid+"\n") {
				return a, nil
			}
		}
		return ai.ClassifyAnswer{Choice: "other", Probabilities: map[string]float64{"other": 1}}, nil
	}
}

func pick(choice string, p float64) ai.ClassifyAnswer {
	return ai.ClassifyAnswer{Choice: choice, Probabilities: map[string]float64{choice: p, "other": 1 - p}}
}

func (f *izFixture) find(refs ...dto.ThreadRef) (dto.InboxZeroResult, error) {
	return f.ai.FindInboxZeroCandidates(context.Background(), refs, "iz-1")
}

func reasonsOf(res dto.InboxZeroResult) map[string]string {
	m := map[string]string{}
	for _, c := range res.Candidates {
		m[c.Ref.ThreadID] = c.Reason
	}
	return m
}

// 未設定は全部 true。保存して読み直せる（全部オフも可）。壊れた値は既定、保存値に無いキーはオン。
func TestInboxZeroSettings(t *testing.T) {
	f := newIZFixture(t)
	ctx := t.Context()
	get := func() dto.InboxZeroReasons {
		got, err := f.ai.GetInboxZeroSettings(ctx)
		require.NoError(t, err)
		return got.Reasons
	}
	assert.Equal(t, allReasons, get())
	want := dto.InboxZeroReasons{Ad: true, Replied: true}
	saved, err := f.ai.SetInboxZeroReasons(ctx, want)
	require.NoError(t, err)
	assert.Equal(t, want, saved.Reasons)
	assert.Equal(t, want, get())
	f.setReasons(t, dto.InboxZeroReasons{})
	assert.Equal(t, dto.InboxZeroReasons{}, get())
	for stored, expect := range map[string]dto.InboxZeroReasons{
		`"garbage"`: allReasons, `{"ad":false}`: {Notification: true, Newsletter: true, Replied: true},
	} {
		require.NoError(t, f.Store.SetSetting(ctx, store.SettingInboxZero, stored))
		assert.Equal(t, expect, get(), stored)
	}
}

// 広告・通知・未読のニュースレターが候補になる。other・読んだニュースレター・確率の低いものは外れ、候補は渡した順。
func TestInboxZeroCandidatesByReason(t *testing.T) {
	f := newIZFixture(t)
	f.jevReady(t)
	refs := []dto.ThreadRef{
		f.mail(t, "t-news-unread", "UNREAD"), f.mail(t, "t-ad"), f.mail(t, "t-news-read"),
		f.mail(t, "t-note"), f.mail(t, "t-other"), f.mail(t, "t-lowad"),
	}
	f.answers(map[string]ai.ClassifyAnswer{
		"t-news-unread": pick("newsletter", 0.95), "t-ad": pick("ad", 0.9), "t-news-read": pick("newsletter", 0.99),
		"t-note": pick("notification", 0.85), "t-lowad": pick("ad", 0.5),
	})
	res, err := f.find(refs...)
	require.NoError(t, err)
	assert.Equal(t, 6, res.Examined)
	assert.Zero(t, res.Skipped)
	assert.Equal(t, []dto.InboxZeroCandidate{
		{Ref: refs[0], From: "Sender", Subject: "subject t-news-unread", Reason: "newsletter"},
		{Ref: refs[1], From: "Sender", Subject: "subject t-ad", Reason: "ad"},
		{Ref: refs[3], From: "Sender", Subject: "subject t-note", Reason: "notification"},
	}, res.Candidates)
	assert.Equal(t, []string{testAPIKey}, f.Classifier.Keys)
}

// 確率の下限は 0.8 以上。オフの理由が 1 位なら、2 位がオンでも候補にしない（送る選択肢は常に全部）。
func TestInboxZeroThresholdAndDisabledTopChoice(t *testing.T) {
	f := newIZFixture(t)
	f.jevReady(t)
	a, b := f.mail(t, "t-079"), f.mail(t, "t-080")
	f.answers(map[string]ai.ClassifyAnswer{"t-079": pick("ad", 0.79), "t-080": pick("ad", 0.8)})
	res, err := f.find(a, b)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"t-080": "ad"}, reasonsOf(res))

	f.setReasons(t, dto.InboxZeroReasons{Notification: true, Newsletter: true, Replied: true})
	f.answers(map[string]ai.ClassifyAnswer{"t-080": {Choice: "ad", Probabilities: map[string]float64{"ad": 0.85, "notification": 0.1, "other": 0.05}}})
	n := f.Classifier.Calls()
	res, err = f.find(b)
	require.NoError(t, err)
	assert.Empty(t, res.Candidates)
	assert.Zero(t, res.Skipped)
	require.Equal(t, n+1, f.Classifier.Calls())
	var names []string
	for _, c := range f.Classifier.Inputs[n].Criteria {
		assert.NotEmpty(t, c.Description)
		names = append(names, c.Name)
	}
	assert.Equal(t, []string{"ad", "notification", "newsletter", "other"}, names)
}

// ニュースレターだけをオンにしたとき、読んだスレッドは Jev にも送らない。未読のメッセージが 1 通でもあれば未読。
func TestInboxZeroNewsletterOnlyWhenUnread(t *testing.T) {
	f := newIZFixture(t)
	f.jevReady(t)
	f.setReasons(t, dto.InboxZeroReasons{Newsletter: true})
	read, unread := f.mail(t, "t-read"), f.mail(t, "t-unread", "UNREAD")
	two := f.thread(t, "t-two", msg("m-old", "t-two", 0, "INBOX", "UNREAD"), msg("m-new", "t-two", 1, "INBOX"))
	f.answers(map[string]ai.ClassifyAnswer{"t-read": pick("newsletter", 0.99), "t-unread": pick("newsletter", 0.99), "t-two": pick("newsletter", 0.99)})
	res, err := f.find(read, unread, two)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"t-unread": "newsletter", "t-two": "newsletter"}, reasonsOf(res))
	assert.Equal(t, 2, f.Classifier.Calls(), "読んだスレッドの内容は送らない")
}

// 返信済みは端末内の判定（キー・同意なし、Jev のクライアントも作らない）。SENT ラベル、または差出人が自分（大文字小文字不問）。下書きは数えない。
func TestInboxZeroRepliedDetection(t *testing.T) {
	f := newIZFixture(t)
	f.setReasons(t, dto.InboxZeroReasons{Replied: true})
	bySent := msg("s2", "t-sent", 1, "SENT")
	bySent.From = model.Address{Name: "別名", Email: "alias@elsewhere.example"}
	byFrom := msg("f2", "t-from", 1, "INBOX")
	byFrom.From = model.Address{Email: "ME@A.Example"}
	draft := msg("d2", "t-draft", 1, "DRAFT", "SENT")
	draft.From = model.Address{Email: f.a.Email}
	refs := []dto.ThreadRef{
		f.thread(t, "t-sent", msg("s1", "t-sent", 0, "INBOX"), bySent),
		f.thread(t, "t-from", msg("f1", "t-from", 0, "INBOX"), byFrom),
		f.thread(t, "t-draft", msg("d1", "t-draft", 0, "INBOX"), draft),
		f.mail(t, "t-plain"),
	}
	res, err := f.find(refs...)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"t-sent": "replied", "t-from": "replied"}, reasonsOf(res))
	assert.Equal(t, 4, res.Examined)
	assert.Zero(t, f.Classifier.Created()+f.Classifier.Calls())
	assert.Equal(t, "別名", res.Candidates[0].From, "差出人は下書きを除いた最新のメッセージ")
}

// 返信済みのスレッドは Jev に送らない。返信済みがオフなら Jev の判定に従う。
func TestInboxZeroRepliedThreadsAreNotSentToJev(t *testing.T) {
	f := newIZFixture(t)
	f.jevReady(t)
	sent, plain := f.thread(t, "t-sent", msg("s1", "t-sent", 0, "SENT")), f.mail(t, "t-ad")
	f.answers(map[string]ai.ClassifyAnswer{"t-sent": pick("ad", 0.99), "t-ad": pick("ad", 0.99)})
	res, err := f.find(sent, plain)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"t-sent": "replied", "t-ad": "ad"}, reasonsOf(res))
	require.Equal(t, 1, f.Classifier.Calls())
	assert.Contains(t, f.Classifier.Inputs[0].State, "subject t-ad")

	f.setReasons(t, dto.InboxZeroReasons{Ad: true})
	res, err = f.find(sent)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"t-sent": "ad"}, reasonsOf(res))
}
