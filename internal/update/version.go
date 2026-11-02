package update

import (
	"strconv"
	"strings"
)

// semver は比較に必要な部分だけを持つ解析済みバージョン（ビルドメタデータは無視する）。
type semver struct {
	core [3]uint64
	pre  []string // プレリリース識別子。空なら正式版
}

// IsNewer は latest が current より新しいかを返す（セマンティックバージョニングの優先順位）。
// 先頭の v は許容する。プレリリース付きは同じ数値の正式版より古い。どちらかが解析できなければ false。
func IsNewer(current, latest string) bool {
	c, ok := parseSemver(current)
	if !ok {
		return false
	}
	l, ok := parseSemver(latest)
	if !ok {
		return false
	}
	return compare(l, c) > 0
}

func parseSemver(s string) (semver, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	var v semver
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre := s[i+1:]
		s = s[:i]
		v.pre = strings.Split(pre, ".")
		for _, id := range v.pre {
			if id == "" {
				return semver{}, false
			}
		}
	}
	parts := strings.Split(s, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return semver{}, false
	}
	for i, p := range parts {
		n, ok := parseNumeric(p)
		if !ok {
			return semver{}, false
		}
		v.core[i] = n
	}
	return v, true
}

// parseNumeric は ASCII 数字のみの非負整数を解析する（符号・空白は不可）。
func parseNumeric(s string) (uint64, bool) {
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(s, 10, 64)
	return n, err == nil
}

func compare(a, b semver) int {
	for i := range a.core {
		if c := cmpUint(a.core[i], b.core[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(a.pre) == 0 && len(b.pre) == 0:
		return 0
	case len(a.pre) == 0:
		return 1
	case len(b.pre) == 0:
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		if c := comparePre(a.pre[i], b.pre[i]); c != 0 {
			return c
		}
	}
	return cmpUint(uint64(len(a.pre)), uint64(len(b.pre)))
}

// comparePre はプレリリース識別子 1 つの比較。数字のみは数値比較で、英字を含むものより小さい。
func comparePre(a, b string) int {
	an, aNum := parseNumeric(a)
	bn, bNum := parseNumeric(b)
	switch {
	case aNum && bNum:
		return cmpUint(an, bn)
	case aNum:
		return -1
	case bNum:
		return 1
	}
	return strings.Compare(a, b)
}

func cmpUint(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
