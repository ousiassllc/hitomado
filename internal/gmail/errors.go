package gmail

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"

	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
)

// retryClass は失敗の再試行方針。
type retryClass int

const (
	retryNone    retryClass = iota // 再試行しない
	retryLimited                   // 429／レート制限の 403。バックオフ後に再試行
	retryServer                    // 5xx。バックオフ後に再試行（冪等でない呼び出しは除く）
	retryAuth                      // 401。トークンを破棄して 1 回だけ再試行
)

// failure は分類済みの失敗。kind は公開 sentinel、status は HTTP ステータス（無ければ 0）。
type failure struct {
	kind   error
	retry  retryClass
	status int
}

// classify は Gmail API／トークン更新／トランスポートのエラーを sentinel に分類する。
// 元エラーの本文は機密を含み得るので、戻り値のエラー文言には使わない。
func classify(err error) failure {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) {
		status := 0
		if re.Response != nil {
			status = re.Response.StatusCode
		}
		switch {
		case re.ErrorCode == "invalid_grant":
			return failure{kind: ErrNeedsReauth, status: status}
		case status == http.StatusTooManyRequests:
			return failure{kind: ErrRateLimited, retry: retryLimited, status: status}
		case status >= 500:
			return failure{kind: ErrGeneric, retry: retryServer, status: status}
		}
		return failure{kind: ErrGeneric, status: status}
	}
	var ge *googleapi.Error
	if errors.As(err, &ge) {
		return classifyHTTP(ge)
	}
	if isNetworkError(err) {
		return failure{kind: ErrOffline}
	}
	return failure{kind: ErrGeneric}
}

func classifyHTTP(ge *googleapi.Error) failure {
	f := failure{kind: ErrGeneric, status: ge.Code}
	switch {
	case ge.Code == http.StatusUnauthorized:
		f.kind, f.retry = ErrNeedsReauth, retryAuth
	case ge.Code == http.StatusNotFound:
		f.kind = ErrNotFound
	case ge.Code == http.StatusTooManyRequests || (ge.Code == http.StatusForbidden && isRateLimitReason(ge)):
		f.kind, f.retry = ErrRateLimited, retryLimited
	case ge.Code >= 500:
		f.retry = retryServer
	}
	return f
}

func isRateLimitReason(ge *googleapi.Error) bool {
	for _, item := range ge.Errors {
		if item.Reason == "rateLimitExceeded" || item.Reason == "userRateLimitExceeded" {
			return true
		}
	}
	return false
}

// isNetworkError は接続不能・名前解決失敗・タイムアウト・途中切断など、ネットワーク不通とみなせるエラーを判定する。
func isNetworkError(err error) bool {
	var ue *url.Error
	var ne net.Error
	return errors.As(err, &ue) || errors.As(err, &ne) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

// resultUnknown は、リクエストを送った後に応答を受け取れなかった可能性のある失敗（切断・タイムアウト・ctx の期限切れ）か。
// 5xx など応答を受け取った失敗は含まない。
func resultUnknown(err error) bool {
	return errors.Is(err, ErrOffline) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

// wrapFailure は sentinel を %w で包んだ公開用エラーを作る（元エラーの本文は含めない）。
func wrapFailure(op string, f failure) error {
	if f.status != 0 {
		return fmt.Errorf("gmail: %s (HTTP %d): %w", op, f.status, f.kind)
	}
	return fmt.Errorf("gmail: %s: %w", op, f.kind)
}

// ctxError は呼び出し側のキャンセル・期限切れ。sentinel には分類せず ctx のエラーをそのまま包む。
func ctxError(ctx context.Context, op string) error {
	return fmt.Errorf("gmail: %s: %w", op, ctx.Err())
}
