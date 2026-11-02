package contacts

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ousiassllc/hitomado/internal/domain/model"
)

func TestNormalize(t *testing.T) {
	assert.Equal(t, "a@b.com", Normalize("  A@B.Com\t"))
	assert.Equal(t, "", Normalize("   "))
}

func TestIsValid(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"alice@example.com", true},
		{"a.b+tag@sub.example.co.jp", true},
		{"Alice@Example.COM", true},
		{"", false},
		{"alice", false},
		{"alice@", false},
		{"@example.com", false},
		{"alice@@example.com", false},
		{"Alice <alice@example.com>", false},
		{"<alice@example.com>", false},
		{" alice@example.com", false},
		{"alice@example.com ", false},
		{"alice@example.com,bob@example.com", false},
		{"alice@example.com\r\nBcc: x@y.com", false},
		{"a b@example.com", false},
		{"\"a b\"@example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, IsValid(tt.in))
		})
	}
}

func TestIsValidTooLong(t *testing.T) {
	long := make([]byte, 250)
	for i := range long {
		long[i] = 'a'
	}
	assert.False(t, IsValid(string(long)+"@example.com"))
}

func TestScore(t *testing.T) {
	assert.Equal(t, 0, Score(0, 0))
	assert.Equal(t, 3, Score(1, 0))
	assert.Equal(t, 1, Score(0, 1))
	assert.Equal(t, 13, Score(4, 1))
}

func cand(email string, send, recv int, last int64) Candidate {
	return Candidate{Address: model.Address{Email: email}, SendCount: send, ReceiveCount: recv, LastUsedAt: last}
}

func emails(as []model.Address) []string {
	out := []string{}
	for _, a := range as {
		out = append(out, a.Email)
	}
	return out
}

func TestRank(t *testing.T) {
	cs := []Candidate{
		cand("low@x.com", 0, 1, 900),
		cand("me@x.com", 9, 9, 999),
		cand("tie-old@x.com", 1, 0, 100),
		cand("high@x.com", 2, 1, 10),
		cand("tie-new@x.com", 0, 3, 200),
		cand("Dup@x.com", 0, 0, 1),
		cand("dup@x.com", 5, 5, 1),
		cand("", 9, 9, 9),
	}
	got := Rank(cs, "ME@x.com", 0)
	assert.Equal(t, []string{"high@x.com", "tie-new@x.com", "tie-old@x.com", "low@x.com", "dup@x.com"}, emails(got))

	assert.Equal(t, []string{"high@x.com", "tie-new@x.com"}, emails(Rank(cs, "me@x.com", 2)))
}

func TestRankStableAndEmpty(t *testing.T) {
	got := Rank([]Candidate{cand("a@x.com", 1, 0, 5), cand("b@x.com", 1, 0, 5)}, "", 0)
	assert.Equal(t, []string{"a@x.com", "b@x.com"}, emails(got))

	empty := Rank(nil, "me@x.com", 10)
	assert.NotNil(t, empty)
	assert.Empty(t, empty)
}
