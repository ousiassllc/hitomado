package appcore

import "strings"

// windowsReserved は Windows の予約デバイス名（拡張子を除いた名前）。
var windowsReserved = map[string]struct{}{
	"CON": {}, "PRN": {}, "AUX": {}, "NUL": {},
	"COM1": {}, "COM2": {}, "COM3": {}, "COM4": {}, "COM5": {}, "COM6": {}, "COM7": {}, "COM8": {}, "COM9": {},
	"LPT1": {}, "LPT2": {}, "LPT3": {}, "LPT4": {}, "LPT5": {}, "LPT6": {}, "LPT7": {}, "LPT8": {}, "LPT9": {},
}

const maxFilenameRunes = 200

// SanitizeFilename は OS の禁止文字（< > : " / \ | ? * と制御文字）を _ に置き換え、
// 末尾の空白・ドット、予約名、空文字を安全な名前にする。
func SanitizeFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	if r := []rune(name); len(r) > maxFilenameRunes {
		name = string(r[:maxFilenameRunes])
	}
	name = strings.TrimRight(strings.TrimSpace(name), ". ")
	if name == "" {
		return "attachment"
	}
	base, _, _ := strings.Cut(name, ".")
	if _, bad := windowsReserved[strings.ToUpper(strings.TrimSpace(base))]; bad {
		return "_" + name
	}
	return name
}
