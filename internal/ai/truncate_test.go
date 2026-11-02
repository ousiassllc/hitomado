package ai_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
)

func msgs(texts ...string) []ai.ContextMessage {
	out := make([]ai.ContextMessage, len(texts))
	for i, t := range texts {
		out[i] = ai.ContextMessage{From: "f", Text: t}
	}
	return out
}

func texts(in []ai.ContextMessage) []string {
	out := make([]string, len(in))
	for i, m := range in {
		out[i] = m.Text
	}
	return out
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		name        string
		in          []ai.ContextMessage
		maxMessages int
		maxChars    int
		want        []string
	}{
		{"all fit", msgs("a", "b", "c"), 5, 100, []string{"a", "b", "c"}},
		{"message count drops oldest", msgs("a", "b", "c", "d"), 2, 100, []string{"c", "d"}},
		{"exactly max messages", msgs("a", "b"), 2, 100, []string{"a", "b"}},
		{"exactly max chars", msgs("aa", "bb"), 5, 4, []string{"aa", "bb"}},
		{"chars drop oldest", msgs("aaaa", "bb", "cc"), 5, 5, []string{"bb", "cc"}},
		{"stops at first that does not fit even if older is small", msgs("a", "bbbbbb", "cc"), 5, 5, []string{"cc"}},
		{"newest alone too long is cut to head", msgs("old", "あいうえおかきく"), 5, 3, []string{"あいう"}},
		{"runes not bytes", msgs("あいう", "えお"), 5, 5, []string{"あいう", "えお"}},
		{"empty input", nil, 5, 100, []string{}},
		{"zero max messages", msgs("a"), 0, 100, []string{}},
		{"zero max chars", msgs("a"), 5, 0, []string{}},
		{"empty texts are kept", msgs("a", "", "b"), 5, 2, []string{"a", "", "b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ai.Truncate(c.in, c.maxMessages, c.maxChars)
			require.NotNil(t, got)
			assert.Equal(t, c.want, texts(got))
		})
	}
}

func TestTruncateDoesNotMutateInput(t *testing.T) {
	in := msgs("old", strings.Repeat("x", 10))
	_ = ai.Truncate(in, 5, 3)
	assert.Equal(t, strings.Repeat("x", 10), in[1].Text)
	assert.Len(t, in, 2)
}

func TestTruncatePreservesOrderAndFields(t *testing.T) {
	in := []ai.ContextMessage{{From: "1", Text: "a"}, {From: "2", Text: "b"}, {From: "3", Text: "c"}}
	got := ai.Truncate(in, 2, 100)
	require.Len(t, got, 2)
	assert.Equal(t, "2", got[0].From)
	assert.Equal(t, "3", got[1].From)
}
