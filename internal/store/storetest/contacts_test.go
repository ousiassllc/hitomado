package storetest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/store"
)

func emails(cs []store.Contact) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Email)
	}
	return out
}

func addrs(emails ...string) []model.Address {
	out := make([]model.Address, 0, len(emails))
	for _, e := range emails {
		out = append(out, model.Address{Email: e})
	}
	return out
}

func TestContactsRanking(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "me@example.com")
	t0 := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

	// 順位: send*3+receive。bob=1*3=3、carol=3、dave=2、erin=1。
	require.NoError(t, s.AddSentContacts(ctx, a.ID, addrs("bob@x.com"), t0))
	for i := range 3 {
		require.NoError(t, s.AddReceivedContacts(ctx, a.ID, addrs("carol@x.com"), t0.Add(time.Duration(i+1)*time.Minute)))
	}
	for i := range 2 {
		require.NoError(t, s.AddReceivedContacts(ctx, a.ID, addrs("dave@x.com"), t0.Add(time.Duration(i)*time.Minute)))
	}
	require.NoError(t, s.AddReceivedContacts(ctx, a.ID, addrs("erin@x.com"), t0.Add(time.Hour)))

	got, err := s.SuggestContacts(ctx, a.ID, "", 10)
	require.NoError(t, err)
	// bob と carol は同点（3）。last_used_at は carol(+3m) が bob(0) より新しい。
	assert.Equal(t, []string{"carol@x.com", "bob@x.com", "dave@x.com", "erin@x.com"}, emails(got))

	// 送信を重ねると carol を抜く。
	require.NoError(t, s.AddSentContacts(ctx, a.ID, addrs("bob@x.com"), t0))
	got, err = s.SuggestContacts(ctx, a.ID, "", 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"bob@x.com", "carol@x.com"}, emails(got), "limit が効く")

	// last_used_at は巻き戻らない。
	var last int64
	require.NoError(t, s.raw.QueryRow(`SELECT last_used_at FROM contacts WHERE email = 'bob@x.com'`).Scan(&last))
	assert.Equal(t, t0.UnixMilli(), last)
	require.NoError(t, s.AddReceivedContacts(ctx, a.ID, addrs("bob@x.com"), t0.Add(-time.Hour)))
	require.NoError(t, s.raw.QueryRow(`SELECT last_used_at FROM contacts WHERE email = 'bob@x.com'`).Scan(&last))
	assert.Equal(t, t0.UnixMilli(), last)
}

func TestContactsCounters(t *testing.T) {
	s, clk := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "me@example.com")
	at := clk.Now()

	// 正規化（trim＋小文字）と、1 回の呼び出し内の重複除去。空アドレスは無視。
	require.NoError(t, s.AddReceivedContacts(ctx, a.ID, []model.Address{
		{Email: " Bob@X.com ", Name: "Bob"}, {Email: "bob@x.com"}, {Email: ""}, {Email: "  "},
	}, at))
	require.NoError(t, s.AddSentContacts(ctx, a.ID, addrs("BOB@x.com"), at))
	require.NoError(t, s.AddSentContacts(ctx, a.ID, nil, at))
	var send, recv int
	var name *string
	require.NoError(t, s.raw.QueryRow(`SELECT send_count, receive_count, display_name FROM contacts WHERE email = 'bob@x.com'`).Scan(&send, &recv, &name))
	assert.Equal(t, 1, send)
	assert.Equal(t, 1, recv)
	require.NotNil(t, name)
	assert.Equal(t, "Bob", *name)
	assert.Equal(t, 1, count(t, s, `SELECT COUNT(*) FROM contacts`))

	// 空の表示名は既存の表示名を消さず、新しい表示名は上書きする。
	require.NoError(t, s.AddReceivedContacts(ctx, a.ID, addrs("bob@x.com"), at))
	got, err := s.SuggestContacts(ctx, a.ID, "bob", 5)
	require.NoError(t, err)
	assert.Equal(t, []store.Contact{{Email: "bob@x.com", DisplayName: "Bob"}}, got)
	require.NoError(t, s.AddReceivedContacts(ctx, a.ID, []model.Address{{Email: "bob@x.com", Name: "Robert"}}, at))
	got, _ = s.SuggestContacts(ctx, a.ID, "bob", 5)
	assert.Equal(t, "Robert", got[0].DisplayName)

	assert.ErrorIs(t, s.AddReceivedContacts(ctx, "missing", addrs("p@x.com"), at), store.ErrNotFound)
}

func TestSuggestContactsPrefixRange(t *testing.T) {
	s, clk := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "me@example.com")
	at := clk.Now()
	require.NoError(t, s.AddReceivedContacts(ctx, a.ID, []model.Address{
		{Email: "alice@x.com"}, {Email: "alicia@x.com"}, {Email: "ali@x.com"}, {Email: "bob@x.com", Name: "Alison Smith"},
		{Email: "a_b@x.com"}, {Email: "axb@x.com"}, {Email: "zed@x.com"},
	}, at))

	got, err := s.SuggestContacts(ctx, a.ID, "ali", 10)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"alice@x.com", "alicia@x.com", "ali@x.com", "bob@x.com"}, emails(got), "メール前方一致＋表示名前方一致")

	got, err = s.SuggestContacts(ctx, a.ID, "  ALIC", 10)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"alice@x.com", "alicia@x.com"}, emails(got), "大文字・前後空白は正規化")

	got, err = s.SuggestContacts(ctx, a.ID, "a_", 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"a_b@x.com"}, emails(got), "LIKE のワイルドカードとして解釈しない")

	got, err = s.SuggestContacts(ctx, a.ID, "nomatch", 10)
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.NotNil(t, got)
}

func TestSuggestContactsExcludesSelfAndIsolatesAccounts(t *testing.T) {
	s, clk := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "me@a.example")
	b := seedAccount(t, s, "me@b.example")
	at := clk.Now()

	// 自アカウントのアドレス（a 自身、および同じ利用者の別アカウント b のアドレスは除外対象ではない）。
	require.NoError(t, s.AddReceivedContacts(ctx, a.ID, addrs("me@a.example", "me@b.example", "x@client.example"), at))
	require.NoError(t, s.AddSentContacts(ctx, a.ID, addrs("me@a.example"), at))
	require.NoError(t, s.AddReceivedContacts(ctx, b.ID, addrs("y@other.example", "me@a.example"), at))

	got, err := s.SuggestContacts(ctx, a.ID, "", 10)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"me@b.example", "x@client.example"}, emails(got), "a の候補から a 自身を除外し、b の候補は混ざらない")

	got, err = s.SuggestContacts(ctx, a.ID, "me@a", 10)
	require.NoError(t, err)
	assert.Empty(t, got)

	got, err = s.SuggestContacts(ctx, b.ID, "", 10)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"y@other.example", "me@a.example"}, emails(got))
}
