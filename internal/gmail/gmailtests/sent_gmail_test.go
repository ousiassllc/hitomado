package gmailtests

import (
	"bytes"
	"net"
	"net/http"
	"net/mail"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/gmail/gmailtest"
)

// sentGmailOpt は偽 Gmail の送信時の挙動。
type sentGmailOpt int

const (
	// gmailRewritesMessageID は送信済みの Message-ID を Gmail 自身の値に付け替える（実 Gmail で報告されている挙動）。
	gmailRewritesMessageID sentGmailOpt = 1 << iota
	// gmailDropsCustomHeaders は X- で始まる独自ヘッダを送信済みに残さない（残る保証が無いことの再現）。
	gmailDropsCustomHeaders
)

type sentMessage struct {
	id      string
	headers [][2]string // 名前, 値
}

// sentGmail は、送信を受理して覚え、messages.list（in:sent）と messages.get（metadata）に答える偽 Gmail。
type sentGmail struct {
	t    *testing.T
	f    *gmailtest.Server
	opts sentGmailOpt

	mu   sync.Mutex
	sent []sentMessage // 新しい順
}

func newSentGmail(t *testing.T, f *gmailtest.Server, opts sentGmailOpt) *sentGmail {
	t.Helper()
	g := &sentGmail{t: t, f: f, opts: opts}
	f.Handle("GET "+listMessages, func(w http.ResponseWriter, _ *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		stubs := []map[string]any{}
		for _, m := range g.sent {
			stubs = append(stubs, map[string]any{"id": m.id, "threadId": "t-" + m.id})
		}
		writeJSON(w, 200, map[string]any{"messages": stubs})
	})
	return g
}

// otherSent は無関係な送信済みメッセージを加える。
func (g *sentGmail) otherSent(id string) {
	g.add(sentMessage{id: id, headers: [][2]string{{"Message-ID", "<other@mail.gmail.com>"}, {"X-Hitomado-Send-Id", "OTHER"}}})
}

// acceptThenDrop は messages.send を受理して覚えたうえで、応答を返さずに接続を切る。
func (g *sentGmail) acceptThenDrop() {
	g.f.Handle("POST "+sendUpload, func(w http.ResponseWriter, r *http.Request) {
		body := new(bytes.Buffer)
		_, _ = body.ReadFrom(r.Body)
		up := gmailtest.ParseUpload(g.t, gmailtest.Request{Query: r.URL.Query(), Header: r.Header, Body: body.Bytes()})
		msg, err := mail.ReadMessage(bytes.NewReader(up.Media))
		if err != nil {
			g.t.Errorf("送信内容を解釈できない: %v", err)
			return
		}
		m := sentMessage{id: "m-sent-1"}
		for name, values := range msg.Header {
			if g.opts&gmailDropsCustomHeaders != 0 && strings.HasPrefix(strings.ToLower(name), "x-") {
				continue
			}
			if g.opts&gmailRewritesMessageID != 0 && strings.EqualFold(name, "Message-ID") {
				values = []string{"<CAKrewritten@mail.gmail.com>"}
			}
			for _, v := range values {
				m.headers = append(m.headers, [2]string{name, v})
			}
		}
		g.add(m)
		dropConnection(w, r)
	})
}

func (g *sentGmail) add(m sentMessage) {
	g.mu.Lock()
	g.sent = slices.Insert(g.sent, 0, m)
	g.mu.Unlock()
	g.f.Handle("GET "+listMessages+"/"+m.id, func(w http.ResponseWriter, r *http.Request) {
		want := r.URL.Query()["metadataHeaders"]
		headers := []map[string]any{}
		for _, h := range m.headers {
			if slices.ContainsFunc(want, func(n string) bool { return strings.EqualFold(n, h[0]) }) {
				headers = append(headers, hdr(h[0], h[1]))
			}
		}
		writeJSON(w, 200, map[string]any{"id": m.id, "threadId": "t-" + m.id, "payload": map[string]any{"headers": headers}})
	})
}

// closedURL は何も待ち受けていないローカルアドレスの URL（接続拒否の再現）。
func closedURL(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return "http://" + addr
}
