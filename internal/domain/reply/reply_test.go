package reply

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ousiassllc/hitomado/internal/domain/model"
)

func addrs(emails ...string) []model.Address {
	out := []model.Address{}
	for _, e := range emails {
		out = append(out, model.Address{Email: e})
	}
	return out
}

func emails(as []model.Address) []string {
	out := []string{}
	for _, a := range as {
		out = append(out, a.Email)
	}
	return out
}

func TestResolve(t *testing.T) {
	const self = "me@example.com"
	tests := []struct {
		name   string
		mode   model.Mode
		src    model.SourceMessage
		wantTo []string
		wantCc []string
	}{
		{
			name:   "返信は From",
			mode:   model.ModeReply,
			src:    model.SourceMessage{From: model.Address{Email: "alice@x.com"}, To: addrs(self, "bob@x.com")},
			wantTo: []string{"alice@x.com"},
			wantCc: []string{},
		},
		{
			name: "返信は Reply-To が優先",
			mode: model.ModeReply,
			src: model.SourceMessage{
				From:    model.Address{Email: "alice@x.com"},
				ReplyTo: addrs("Support@X.com"),
				To:      addrs(self),
			},
			wantTo: []string{"support@x.com"},
			wantCc: []string{},
		},
		{
			name: "返信は複数 Reply-To を重複除去",
			mode: model.ModeReply,
			src: model.SourceMessage{
				From:    model.Address{Email: "alice@x.com"},
				ReplyTo: addrs("a@x.com", "A@x.com", "b@x.com"),
			},
			wantTo: []string{"a@x.com", "b@x.com"},
			wantCc: []string{},
		},
		{
			name: "自分が最新送信者なら元の To（自分は除く）",
			mode: model.ModeReply,
			src: model.SourceMessage{
				From: model.Address{Email: "Me@Example.com"},
				To:   addrs("bob@x.com", self, "carol@x.com"),
				Cc:   addrs("dan@x.com"),
			},
			wantTo: []string{"bob@x.com", "carol@x.com"},
			wantCc: []string{},
		},
		{
			name:   "自分宛のみに送ったメッセージへの返信は自分を残す",
			mode:   model.ModeReply,
			src:    model.SourceMessage{From: model.Address{Email: self}, To: addrs(self)},
			wantTo: []string{self},
			wantCc: []string{},
		},
		{
			name: "全員返信は To/Cc から自分を除き Cc に回す",
			mode: model.ModeReplyAll,
			src: model.SourceMessage{
				From: model.Address{Email: "alice@x.com"},
				To:   addrs(self, "bob@x.com"),
				Cc:   addrs("carol@x.com", "ME@example.com"),
			},
			wantTo: []string{"alice@x.com"},
			wantCc: []string{"bob@x.com", "carol@x.com"},
		},
		{
			name: "全員返信は To に居る人を Cc から除き重複を除去",
			mode: model.ModeReplyAll,
			src: model.SourceMessage{
				From:    model.Address{Email: "alice@x.com"},
				ReplyTo: addrs("list@x.com"),
				To:      addrs("list@x.com", "bob@x.com", "alice@x.com"),
				Cc:      addrs("BOB@x.com", "carol@x.com"),
			},
			wantTo: []string{"list@x.com"},
			wantCc: []string{"bob@x.com", "alice@x.com", "carol@x.com"},
		},
		{
			name: "全員返信で自分が最新送信者なら元の To と Cc",
			mode: model.ModeReplyAll,
			src: model.SourceMessage{
				From: model.Address{Email: self},
				To:   addrs("bob@x.com", "carol@x.com"),
				Cc:   addrs("dan@x.com", "bob@x.com"),
			},
			wantTo: []string{"bob@x.com", "carol@x.com"},
			wantCc: []string{"dan@x.com"},
		},
		{
			name:   "転送は空",
			mode:   model.ModeForward,
			src:    model.SourceMessage{From: model.Address{Email: "alice@x.com"}, To: addrs(self)},
			wantTo: []string{},
			wantCc: []string{},
		},
		{
			name:   "new は空",
			mode:   model.ModeNew,
			src:    model.SourceMessage{},
			wantTo: []string{},
			wantCc: []string{},
		},
		{
			name:   "From が空のアドレスは捨てる",
			mode:   model.ModeReply,
			src:    model.SourceMessage{From: model.Address{Email: " "}},
			wantTo: []string{},
			wantCc: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.mode, tt.src, self)
			assert.Equal(t, tt.wantTo, emails(got.To))
			assert.Equal(t, tt.wantCc, emails(got.Cc))
			assert.NotNil(t, got.To)
			assert.NotNil(t, got.Cc)
		})
	}
}

func TestResolveKeepsDisplayName(t *testing.T) {
	src := model.SourceMessage{From: model.Address{Name: " Alice ", Email: "Alice@X.com"}}
	got := Resolve(model.ModeReply, src, "me@example.com")
	assert.Equal(t, []model.Address{{Name: "Alice", Email: "alice@x.com"}}, got.To)
}

func TestSubject(t *testing.T) {
	tests := []struct {
		mode model.Mode
		orig string
		want string
	}{
		{model.ModeReply, "Hello", "Re: Hello"},
		{model.ModeReply, "Re: Hello", "Re: Hello"},
		{model.ModeReply, "RE: Hello", "RE: Hello"},
		{model.ModeReply, "re:Hello", "re:Hello"},
		{model.ModeReplyAll, "  Hello  ", "Re: Hello"},
		{model.ModeReply, "Fwd: Hello", "Re: Fwd: Hello"},
		{model.ModeReply, "Reminder", "Re: Reminder"},
		{model.ModeReply, "", "Re:"},
		{model.ModeForward, "Hello", "Fwd: Hello"},
		{model.ModeForward, "Fwd: Hello", "Fwd: Hello"},
		{model.ModeForward, "FWD: Hello", "FWD: Hello"},
		{model.ModeForward, "Fw: Hello", "Fw: Hello"},
		{model.ModeForward, "Re: Hello", "Fwd: Re: Hello"},
		{model.ModeForward, "Fwdx", "Fwd: Fwdx"},
		{model.ModeNew, "Hello", "Hello"},
	}
	for _, tt := range tests {
		t.Run(string(tt.mode)+"/"+tt.orig, func(t *testing.T) {
			assert.Equal(t, tt.want, Subject(tt.mode, tt.orig))
		})
	}
}

func TestSubjectFoldsLineBreaks(t *testing.T) {
	assert.Equal(t, "Re: a Bcc: x@y.example", Subject(model.ModeReply, "a\r\nBcc: x@y.example"))
	assert.Equal(t, "Fwd: a b", Subject(model.ModeForward, "\na\n\nb\r"))
	assert.Equal(t, "Re: x", Subject(model.ModeReply, "Re:\r\nx"), "接頭辞があれば重複させず、改行だけ畳む")
	assert.Equal(t, "a b", Subject(model.ModeNew, "a\nb"))
}
