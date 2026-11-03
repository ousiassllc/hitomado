package logging

import (
	"bufio"
	"bytes"
	"errors"
	"io"
)

// eachLine は r を 1 行ずつ fn へ渡す。maxLineBytes を超える行は読み飛ばし、後ろの行は読み続ける。
func eachLine(r io.Reader, fn func(line []byte)) error {
	br := bufio.NewReaderSize(r, 64*1024)
	var buf []byte
	tooLong := false
	for {
		chunk, isPrefix, err := br.ReadLine()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err //nolint:wrapcheck // 呼び出し元がまとめて包む
		}
		if !tooLong {
			if len(buf)+len(chunk) > maxLineBytes {
				tooLong = true
			} else {
				buf = append(buf, chunk...)
			}
		}
		if isPrefix {
			continue
		}
		if !tooLong {
			fn(buf)
		}
		buf, tooLong = buf[:0], false
	}
}

// reverseChunkBytes は eachLineReverse が 1 回に末尾から読む大きさ。
const reverseChunkBytes = 64 * 1024

// eachLineReverse は r（先頭から size バイト）の行を末尾から先頭へ向かって fn へ渡す。fn が false を返したら止める。
// 行末の \r\n の \r は外す。末尾に改行が無い最後の行も 1 行として渡す。空行と、maxLineBytes を超える行は飛ばす（eachLine と同じ）。
// fn に渡す line は次の呼び出しまでしか有効でない。
func eachLineReverse(r io.ReaderAt, size int64, fn func(line []byte) bool) error {
	buf := make([]byte, reverseChunkBytes)
	var carry []byte // いま組み立て中の行の、読み終えた後ろ側（先頭側はまだ前のチャンクにある）
	tooLong := false
	emit := func(head []byte) bool {
		line := head
		if len(carry) > 0 {
			line = append(append(make([]byte, 0, len(head)+len(carry)), head...), carry...)
		}
		line = bytes.TrimSuffix(line, []byte("\r"))
		carry = carry[:0]
		return len(line) == 0 || fn(line)
	}
	for pos := size; pos > 0; {
		n := int(min(int64(len(buf)), pos))
		pos -= int64(n)
		chunk := buf[:n]
		if _, err := r.ReadAt(chunk, pos); err != nil && !errors.Is(err, io.EOF) {
			return err //nolint:wrapcheck // 呼び出し元がまとめて包む
		}
		for end := n; ; {
			i := bytes.LastIndexByte(chunk[:end], '\n')
			head := chunk[i+1 : end] // i < 0 のときはチャンクの先頭から（この行の先頭は前のチャンクにある）
			switch {
			case tooLong:
			case len(head)+len(carry) > maxLineBytes:
				tooLong, carry = true, carry[:0]
			case i < 0:
				carry = append(append(make([]byte, 0, len(head)+len(carry)), head...), carry...)
			}
			if i < 0 {
				break
			}
			if !tooLong && !emit(head) {
				return nil
			}
			tooLong, carry, end = false, carry[:0], i
		}
	}
	if !tooLong && len(carry) > 0 {
		emit(nil)
	}
	return nil
}
