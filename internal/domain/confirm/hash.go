package confirm

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"
	"strconv"
	"strings"

	"github.com/ousiassllc/hitomado/internal/domain/model"
)

// Hash は送信内容の SHA-256。
type Hash [32]byte

// AttachmentRef は添付の識別情報（ID とサイズ）。
type AttachmentRef struct {
	ID   string
	Size int64
}

// ContentInput は確認時と送信時で同一性を検証する内容。送信元は AccountID のみから決まる。
type ContentInput struct {
	AccountID   string
	To, Cc, Bcc []model.Address
	Subject     string
	BodyHTML    string
	Attachments []AttachmentRef
	Archive     bool
}

// HashContent は内容を正規化して SHA-256 を取る。アドレスは小文字化・前後空白除去
// （順序は保持）、添付は ID 順（同一 ID はサイズ順）に並べてから計算する。
// フィールド境界が曖昧にならないよう、各値は長さ付きで書き込む。
func HashContent(in ContentInput) Hash {
	h := sha256.New()
	str := func(s string) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(s)))
		h.Write(n[:])
		h.Write([]byte(s))
	}
	num := func(v uint64) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], v)
		h.Write(n[:])
	}
	addrs := func(as []model.Address) {
		num(uint64(len(as)))
		for _, a := range as {
			str(strings.TrimSpace(a.Name))
			str(strings.ToLower(strings.TrimSpace(a.Email)))
		}
	}

	str(in.AccountID)
	addrs(in.To)
	addrs(in.Cc)
	addrs(in.Bcc)
	str(in.Subject)
	str(in.BodyHTML)

	atts := append([]AttachmentRef(nil), in.Attachments...)
	sort.Slice(atts, func(i, j int) bool {
		if atts[i].ID != atts[j].ID {
			return atts[i].ID < atts[j].ID
		}
		return atts[i].Size < atts[j].Size
	})
	num(uint64(len(atts)))
	for _, a := range atts {
		str(a.ID)
		str(strconv.FormatInt(a.Size, 10))
	}
	if in.Archive {
		num(1)
	} else {
		num(0)
	}

	var out Hash
	copy(out[:], h.Sum(nil))
	return out
}
