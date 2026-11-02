package contacts

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
