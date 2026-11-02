package appcore

import (
	"strings"
	"unicode/utf8"
)

// windowsReserved は Windows の予約デバイス名（拡張子を除いた名前）。
var windowsReserved = map[string]struct{}{
	"CON": {}, "PRN": {}, "AUX": {}, "NUL": {},
	"COM1": {}, "COM2": {}, "COM3": {}, "COM4": {}, "COM5": {}, "COM6": {}, "COM7": {}, "COM8": {}, "COM9": {},
	"LPT1": {}, "LPT2": {}, "LPT3": {}, "LPT4": {}, "LPT5": {}, "LPT6": {}, "LPT7": {}, "LPT8": {}, "LPT9": {},
}

const (
	// maxFilenameBytes は NTFS／ext4 のファイル名上限（バイト。UTF-8 で数える）。
	maxFilenameBytes = 255
	// maxExtBytes はこれより長い「拡張子」は拡張子として保持しない（本体の切り詰めを潰さないため）。
	maxExtBytes = 32
)

// SanitizeFilename は OS の禁止文字（< > : " / \ | ? * と制御文字）を _ に置き換え、
// 末尾の空白・ドット、予約名、空文字を安全な名前にする。長さは 255 バイト以内で、拡張子は保つ（本体側を切る）。
func SanitizeFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == utf8.RuneError || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.ToValidUTF8(strings.TrimRight(strings.TrimSpace(name), ". "), "_")
	if name == "" {
		return "attachment"
	}
	name = truncateKeepingExt(name)
	base, _, _ := strings.Cut(name, ".")
	if _, bad := windowsReserved[strings.ToUpper(strings.TrimSpace(base))]; bad {
		return "_" + name // 先頭に 1 バイト足しても上限を超えないよう、切り詰めは予約名判定の前後で余裕を見る
	}
	return name
}

// truncateKeepingExt は name を maxFilenameBytes-1 バイト（予約名の接頭辞 1 バイト分の余裕）以内にする。
func truncateKeepingExt(name string) string {
	const limit = maxFilenameBytes - 1
	if len(name) <= limit {
		return name
	}
	ext := ""
	if i := strings.LastIndexByte(name, '.'); i > 0 && len(name)-i <= maxExtBytes {
		ext = name[i:]
	}
	base := cutBytes(name[:len(name)-len(ext)], limit-len(ext))
	base = strings.TrimRight(base, ". ")
	if base == "" {
		base = "attachment"
	}
	return base + ext
}

// cutBytes は s を n バイト以内で、ルーン境界で切る。
func cutBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
