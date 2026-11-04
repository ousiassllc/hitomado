package secrets

import "strings"

// foldCase はキーの大文字小文字を区別しない Store（Windows 資格情報マネージャーと同じ振る舞い）。
type foldCase struct{ Store }

// FoldKeyCase は s を、キーの大文字小文字を区別しない Store として包む。テスト用:
// Memory は区別するので、Windows でだけ起きる「大文字小文字だけが違うキーが同じ値になる」状況をほかの OS で再現するのに使う。
func FoldKeyCase(s Store) Store { return foldCase{s} }

func (f foldCase) Get(key string) (string, error) { return f.Store.Get(strings.ToLower(key)) }
func (f foldCase) Set(key, value string) error    { return f.Store.Set(strings.ToLower(key), value) }
func (f foldCase) Delete(key string) error        { return f.Store.Delete(strings.ToLower(key)) }
