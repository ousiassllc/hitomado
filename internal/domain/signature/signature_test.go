package signature

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBlock(t *testing.T) {
	tests := []struct {
		name, account, sig, want string
	}{
		{"基本", "a1", "<p>Taro</p>", `<div data-hitomado-signature data-account-id="a1">-- <br><p>Taro</p></div>`},
		{"空", "a1", "", ""},
		{"空白のみ", "a1", " \n ", ""},
		{"許可タグが残らない署名", "a1", "<script>alert(1)</script>", ""},
		{"署名はサニタイズされる", "a1", `Taro<img src=x onerror=alert(1)><script>x</script>`,
			`<div data-hitomado-signature data-account-id="a1">-- <br>Taro</div>`},
		{"アカウント ID は属性エスケープ", "a\"><script>", "x",
			`<div data-hitomado-signature data-account-id="a&#34;&gt;&lt;script&gt;">-- <br>x</div>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Block(tt.account, tt.sig))
		})
	}
}

func TestInsert(t *testing.T) {
	blockA1 := `<div data-hitomado-signature data-account-id="a1">-- <br>Taro</div>`
	blockA2 := `<div data-hitomado-signature data-account-id="a2">-- <br>Taro</div>`
	quote := `<blockquote data-hitomado-quote><p>q</p></blockquote>`
	tests := []struct {
		name, body, account, sig, want string
	}{
		{"末尾に挿入", "<p>hi</p>", "a1", "Taro", "<p>hi</p>" + blockA1},
		{"本文が空", "", "a1", "Taro", blockA1},
		{"引用の直前に挿入", "<p>hi</p>" + quote, "a1", "Taro", "<p>hi</p>" + blockA1 + quote},
		{"引用のみ", quote, "a1", "Taro", blockA1 + quote},
		{"最初の引用の直前", quote + "x" + quote, "a1", "Taro", blockA1 + quote + "x" + quote},
		{"data-hitomado-quote 以外の blockquote は無視", "<blockquote>x</blockquote>", "a1", "Taro",
			"<blockquote>x</blockquote>" + blockA1},
		{"テキスト中の文字列は引用とみなさない", "&lt;blockquote data-hitomado-quote&gt;", "a1", "Taro",
			"&lt;blockquote data-hitomado-quote&gt;" + blockA1},
		{"同アカウントの署名があれば二重挿入しない", "<p>hi</p>" + blockA1, "a1", "Taro", "<p>hi</p>" + blockA1},
		{"同アカウント署名＋引用でも二重挿入しない", "<p>hi</p>" + blockA1 + quote, "a1", "Taro", "<p>hi</p>" + blockA1 + quote},
		{"別アカウントの署名は別ブロックとして挿入", "<p>hi</p>" + blockA2, "a1", "Taro", "<p>hi</p>" + blockA2 + blockA1},
		{"署名が空なら無変更", "<p>hi</p>", "a1", "", "<p>hi</p>"},
		{"署名が空でも引用はそのまま", quote, "a1", " ", quote},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Insert(tt.body, tt.account, tt.sig))
		})
	}
}

func TestInsertIsIdempotent(t *testing.T) {
	once := Insert("<p>hi</p><blockquote data-hitomado-quote>q</blockquote>", "a1", "<b>Taro</b>")
	assert.Equal(t, once, Insert(once, "a1", "<b>Taro</b>"))
}

func TestInsertQuoteWithUppercaseTagAndSingleQuotes(t *testing.T) {
	body := "<p>x</p><BLOCKQUOTE DATA-HITOMADO-QUOTE='1'>q</BLOCKQUOTE>"
	got := Insert(body, "a1", "S")
	assert.Equal(t, "<p>x</p>"+Block("a1", "S")+"<BLOCKQUOTE DATA-HITOMADO-QUOTE='1'>q</BLOCKQUOTE>", got)
}
