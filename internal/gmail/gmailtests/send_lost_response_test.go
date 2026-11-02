package gmailtests

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/gmail"
)

const (
	listMessages = apiBase + "/messages"
	sentRaw      = "From: a@example.com\r\nMessage-ID: <01ABC@example.com>\r\nX-Hitomado-Send-Id: 01ABC\r\n\r\nbody"
)

// dropConnection は応答を返さずに接続を切る（サーバー側で処理できたが応答だけが失われた状況の再現）。
func dropConnection(w http.ResponseWriter, _ *http.Request) {
	conn, _, err := w.(http.Hijacker).Hijack()
	if err == nil {
		_ = conn.Close()
	}
}

func TestTransferCallsAreNotCutByClientTimeout(t *testing.T) {
	const clientTimeout = 50 * time.Millisecond
	const uploadTime = 4 * clientTimeout // 通常呼び出しなら打ち切られる長さ
	e := newTestEnvWith(t, func(o *gmail.Options) { o.HTTPClient = &http.Client{Timeout: clientTimeout} })
	slow := func(status int, body map[string]any) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(uploadTime)
			writeJSON(w, status, body)
		}
	}
	e.f.Handle("POST "+sendUpload, slow(200, map[string]any{"id": "sent-1", "threadId": "t1"}))
	e.f.Handle("GET "+apiBase+"/profile", slow(200, map[string]any{"emailAddress": "a@example.com", "historyId": "1"}))

	got, err := e.c.Send(context.Background(), testAccount, []byte(sentRaw), "t1")
	require.NoError(t, err, "転送系の呼び出しには総時間の上限を掛けない")
	assert.Equal(t, gmail.SentMessage{MessageID: "sent-1", ThreadID: "t1"}, got)

	_, err = e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, gmail.ErrOffline, "通常の呼び出しにはタイムアウトが掛かる")
}

func TestTransferCallsStillHonorContextDeadline(t *testing.T) {
	e := newTestEnv(t)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	e.f.Handle("POST "+sendUpload, func(http.ResponseWriter, *http.Request) { <-release })
	e.f.HandleJSON("GET "+listMessages, 200, map[string]any{})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := e.c.Send(ctx, testAccount, []byte(sentRaw), "")
	require.ErrorIs(t, err, gmail.ErrSendUncertain, "送ったあとの期限切れは、送信済みを確認できなければ確認不能")
}

func TestSendWithCanceledContextBeforeRequestIsNotUncertain(t *testing.T) {
	e := newTestEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := e.c.Send(ctx, testAccount, []byte(sentRaw), "")
	require.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, gmail.ErrSendUncertain, "リクエストを送っていないので確認不能にしない")
	assert.Empty(t, e.f.Requests("POST", sendUpload))
}

// Gmail が送信時に Message-ID を付け替えても、独自ヘッダで受理済みを見つけて成功として返す（二重送信にしない）。
func TestSendLostResponseIsSuccessWhenSendIDHeaderSurvives(t *testing.T) {
	e := newTestEnv(t)
	gm := newSentGmail(t, e.f, gmailRewritesMessageID)
	gm.otherSent("m-other") // 無関係な送信済み。取り違えない
	gm.acceptThenDrop()

	got, err := e.c.Send(context.Background(), testAccount, []byte(sentRaw), "")
	require.NoError(t, err)
	assert.Equal(t, gmail.SentMessage{MessageID: "m-sent-1", ThreadID: "t-m-sent-1"}, got)
	assert.Len(t, e.f.Requests("POST", sendUpload), 1, "再送しない")
	assert.Regexp(t, `^in:sent after:\d+$`, e.f.Requests("GET", listMessages)[0].Query.Get("q"))
}

// Message-ID が書き換えられず、独自ヘッダが落とされた場合でも Message-ID の一致で見つける。
func TestSendLostResponseIsSuccessWhenMessageIDSurvives(t *testing.T) {
	e := newTestEnv(t)
	gm := newSentGmail(t, e.f, gmailDropsCustomHeaders)
	gm.acceptThenDrop()

	got, err := e.c.Send(context.Background(), testAccount, []byte(sentRaw), "")
	require.NoError(t, err)
	assert.Equal(t, "m-sent-1", got.MessageID)
}

// Message-ID を付け替えられ、独自ヘッダも残らないなら見つけられない。未送信とは言えないので再送へ誘導せず SEND_UNCERTAIN。
func TestSendLostResponseWithoutAnyMarkerIsUncertain(t *testing.T) {
	e := newTestEnv(t)
	gm := newSentGmail(t, e.f, gmailRewritesMessageID|gmailDropsCustomHeaders)
	gm.acceptThenDrop()

	_, err := e.c.Send(context.Background(), testAccount, []byte(sentRaw), "")
	require.ErrorIs(t, err, gmail.ErrSendUncertain)
	assert.NotErrorIs(t, err, gmail.ErrOffline, "OFFLINE のまま再送へ誘導しない")
	assert.Len(t, e.f.Requests("POST", sendUpload), 1)
}

// 送信済みに見当たらないだけでは未送信と断定できない（検索インデックスの遅れなど）ので、元のエラーは返さない。
func TestSendLostResponseWithoutMessageIsUncertain(t *testing.T) {
	e := newTestEnv(t)
	e.f.Handle("POST "+sendUpload, dropConnection)
	e.f.HandleJSON("GET "+listMessages, 200, map[string]any{"resultSizeEstimate": 0})

	_, err := e.c.Send(context.Background(), testAccount, []byte(sentRaw), "")
	require.ErrorIs(t, err, gmail.ErrSendUncertain)
	assert.NotErrorIs(t, err, gmail.ErrOffline)
}

func TestSendLostResponseAndFailedLookupIsUncertain(t *testing.T) {
	e := newTestEnv(t)
	e.f.Handle("POST "+sendUpload, dropConnection)
	e.f.Handle("GET "+listMessages, dropConnection)

	_, err := e.c.Send(context.Background(), testAccount, []byte(sentRaw), "")
	require.ErrorIs(t, err, gmail.ErrSendUncertain)
	assert.NotErrorIs(t, err, gmail.ErrOffline, "OFFLINE のまま再送へ誘導しない")
	assert.Len(t, e.f.Requests("POST", sendUpload), 1)
}

func TestSendLostResponseWithoutSendIDIsUncertain(t *testing.T) {
	e := newTestEnv(t)
	e.f.Handle("POST "+sendUpload, dropConnection)

	_, err := e.c.Send(context.Background(), testAccount, []byte("From: a@example.com\r\n\r\nbody"), "")
	require.ErrorIs(t, err, gmail.ErrSendUncertain)
	assert.Empty(t, e.f.Requests("GET", listMessages))
}

func TestSendServerErrorIsNotRetriedNorLookedUp(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("POST "+sendUpload, 503, apiError(503, "backendError", "down"))

	_, err := e.c.Send(context.Background(), testAccount, []byte(sentRaw), "")
	require.ErrorIs(t, err, gmail.ErrGeneric)
	assert.Len(t, e.f.Requests("POST", sendUpload), 1, "5xx は再試行しない")
	assert.Empty(t, e.f.Requests("GET", listMessages), "応答を受け取れた失敗では検索しない")
}

// 接続前の失敗（接続拒否）はリクエストを送れていないので OFFLINE（再送可）。送信済みの確認はしない。
func TestSendConnectionRefusedIsOfflineNotUncertain(t *testing.T) {
	e := newTestEnvWith(t, func(o *gmail.Options) { o.Endpoint = closedURL(t) + "/" })

	_, err := e.c.Send(context.Background(), testAccount, []byte(sentRaw), "")
	require.ErrorIs(t, err, gmail.ErrOffline)
	assert.NotErrorIs(t, err, gmail.ErrSendUncertain)
	assert.Empty(t, e.f.Requests("GET", listMessages), "送っていないので送信済みの確認はしない")
}

// アクセストークンの更新（本文を送る前）に通信できない場合も OFFLINE（再送可）。Gmail へは何も送られない。
func TestSendTokenRefreshNetworkFailureIsOfflineNotUncertain(t *testing.T) {
	e := newTestEnvWith(t, func(o *gmail.Options) { o.TokenURL = closedURL(t) + "/token" })

	_, err := e.c.Send(context.Background(), testAccount, []byte(sentRaw), "")
	require.ErrorIs(t, err, gmail.ErrOffline)
	assert.NotErrorIs(t, err, gmail.ErrSendUncertain)
	assert.Empty(t, e.f.Requests("POST", sendUpload))
	assert.Empty(t, e.f.Requests("GET", listMessages))
}
