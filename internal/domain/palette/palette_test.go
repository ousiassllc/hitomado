package palette

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKeys(t *testing.T) {
	want := []string{"red", "orange", "amber", "green", "teal", "cyan", "blue", "indigo", "purple", "pink"}
	assert.Equal(t, want, Keys())

	got := Keys()
	got[0] = "x"
	assert.Equal(t, "red", Keys()[0], "コピーを返す")
}

func TestIsValid(t *testing.T) {
	assert.True(t, IsValid("teal"))
	assert.False(t, IsValid(""))
	assert.False(t, IsValid("Red"))
	assert.False(t, IsValid("gray"))
}

func TestNextUnused(t *testing.T) {
	all := Keys()
	tests := []struct {
		name string
		used []string
		want string
	}{
		{"空", nil, "red"},
		{"先頭使用済み", []string{"red"}, "orange"},
		{"飛び地", []string{"red", "orange", "green"}, "amber"},
		{"順不同", []string{"orange", "red"}, "amber"},
		{"パレット外は無視", []string{"gray", "", "red"}, "orange"},
		{"最後だけ未使用", all[:9], "pink"},
		{"全使用で先頭へ循環", all, "red"},
		{"全使用＋重複で最少使用", append(append([]string{}, all...), "red"), "orange"},
		{"2 周目の途中", append(append([]string{}, all...), "red", "orange", "amber"), "green"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NextUnused(tt.used))
		})
	}
}
