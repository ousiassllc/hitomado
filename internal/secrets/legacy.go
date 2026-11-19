package secrets

import (
	"errors"
	"strings"
)

const (
	// keyPrefix は現在のキー名の接頭辞（nefos-inbox/<種別>/<ID>）。
	keyPrefix = "nefos-inbox/"
	// legacyKeyPrefix は v0.1.46 までのキー名の接頭辞（hitomado/<種別>/<ID>）。
	legacyKeyPrefix = "hitomado/"
)

// legacyKey は現在のキー名に対応する旧キー名を返す。分割保存の接尾辞（`#<n>`）などは、そのまま引き継ぐ。
func legacyKey(key string) (string, bool) {
	rest, ok := strings.CutPrefix(key, keyPrefix)
	if !ok {
		return "", false
	}
	return legacyKeyPrefix + rest, true
}

type legacyStore struct{ cur, legacy Store }

// WithLegacy は cur を使う Store を返す。cur に無いキーは legacy の旧キー名（hitomado/…）を探し、
// あれば cur へ書き写してから返す（再ログインは要らない）。書き写した値を cur から読み戻して一致を確かめてから、
// legacy 側の値を消す。書き写しに失敗したときは legacy の値を消さずに、読めた値をそのまま返す（次の読み出しでもう一度試す）。
// Delete は両方から消す（消した値が旧キーから戻らないようにする）。
// cur と legacy が同じストア（Android）でも、サービス名だけが違うストア（Windows）でも使える。
func WithLegacy(cur, legacy Store) Store { return legacyStore{cur: cur, legacy: legacy} }

// Get は cur の値を返し、無ければ旧キー名の値を cur へ移して返す。値が無ければ ErrNotFound。
func (s legacyStore) Get(key string) (string, error) {
	v, err := s.cur.Get(key)
	if err == nil || !errors.Is(err, ErrNotFound) || errors.Is(err, ErrKeyLost) {
		return v, err
	}
	lk, ok := legacyKey(key)
	if !ok {
		return "", err
	}
	lv, lerr := s.legacy.Get(lk)
	if lerr != nil {
		if errors.Is(lerr, ErrNotFound) {
			return "", err
		}
		return "", lerr
	}
	if s.cur.Set(key, lv) == nil {
		if back, gerr := s.cur.Get(key); gerr == nil && back == lv {
			_ = s.legacy.Delete(lk) // 移せたあとの掃除。失敗しても次から cur を先に読むので害はない
		}
	}
	return lv, nil
}

// Set は cur に保存する。
func (s legacyStore) Set(key, value string) error { return s.cur.Set(key, value) }

// Delete は cur と旧キー名の両方から消す。無くてもエラーにしない。
func (s legacyStore) Delete(key string) error {
	err := s.cur.Delete(key)
	if lk, ok := legacyKey(key); ok {
		if lerr := s.legacy.Delete(lk); err == nil {
			err = lerr
		}
	}
	return err
}
