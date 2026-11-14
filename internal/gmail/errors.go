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

	"github.com/ousiassllc/hitomado/internal/gmail/googleerr"
	"github.com/ousiassllc/hitomado/internal/gmail/tokenauth"
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
// api は Gmail API が返したエラー応答の情報（応答を受け取っていれば非 nil）。
type failure struct {
	kind   error
	retry  retryClass
	status int
	api    *APIError
	reason string // 再接続が必要な失敗の理由（Reason*）。それ以外は空
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
			return failure{kind: ErrNeedsReauth, status: status, reason: ReasonInvalidGrant}
		case re.ErrorCode == "invalid_client" || re.ErrorCode == "unauthorized_client" || status == http.StatusUnauthorized:
			// Client Secret の無効化・ローテーションなど。再接続や再試行では直らず、OAuth クライアントの設定を直す必要がある。
			return failure{kind: ErrClientRejected, status: status, reason: ReasonClientRejected}
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
	f := failure{kind: ErrGeneric, status: ge.Code, api: &APIError{Info: googleerr.Parse(ge)}}
	switch {
	case ge.Code == http.StatusUnauthorized:
		f.kind, f.retry, f.reason = ErrNeedsReauth, retryAuth, ReasonAPIUnauthorized
	case ge.Code == http.StatusNotFound:
		f.kind = ErrNotFound
	case ge.Code == http.StatusTooManyRequests || (ge.Code == http.StatusForbidden && isRateLimitReason(ge)):
		f.kind, f.retry = ErrRateLimited, retryLimited
	case ge.Code == http.StatusForbidden && f.api.hasReason(scopeInsufficientReasons):
		// 権限（スコープ）が足りないのは、再試行では直らず、再接続して許可し直す必要がある。
		f.kind, f.reason = ErrNeedsReauth, ReasonScopeInsufficient
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

// notSent は、リクエストを Gmail へ送る前に失敗したと言える生のエラーか（トークン更新の失敗・名前解決失敗・接続確立失敗）。
// これらは本文を送れていないので、ネットワーク不通（ErrOffline）のまま再試行してよい。
func notSent(err error) bool {
	var dns *net.DNSError
	var op *net.OpError
	return errors.Is(err, tokenauth.ErrRefresh) || errors.As(err, &dns) || (errors.As(err, &op) && op.Op == "dial")
}

// resultUnknown は、生のエラー（sentinel にする前）が、リクエストを送った後に応答を受け取れなかった
// 可能性のある失敗（切断・タイムアウト・ctx の期限切れ）か。送る前の失敗（notSent）や、
// 5xx など応答を受け取った失敗は含まない。
func resultUnknown(raw error) bool {
	if raw == nil || notSent(raw) {
		return false
	}
	return errors.Is(classify(raw).kind, ErrOffline) || errors.Is(raw, context.DeadlineExceeded) || errors.Is(raw, context.Canceled)
}

// wrapFailure は sentinel を %w で包んだ公開用エラーを作る。Gmail API の応答を受け取った失敗は *APIError にして、
// Google が返した reason・status・message（伏せ字処理済み）を文言に含める。トークン更新などの元エラーの本文は含めない。
func wrapFailure(op string, f failure) error {
	if f.api != nil {
		ae := *f.api
		ae.Op, ae.Kind = op, f.kind
		return WithReason(&ae, f.reason)
	}
	if f.status != 0 {
		return WithReason(fmt.Errorf("gmail: %s (HTTP %d): %w", op, f.status, f.kind), f.reason)
	}
	return WithReason(fmt.Errorf("gmail: %s: %w", op, f.kind), f.reason)
}

// ctxError は呼び出し側のキャンセル・期限切れ。sentinel には分類せず ctx のエラーをそのまま包む。
func ctxError(ctx context.Context, op string) error {
	return fmt.Errorf("gmail: %s: %w", op, ctx.Err())
}
