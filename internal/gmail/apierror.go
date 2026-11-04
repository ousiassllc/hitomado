package gmail

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/ousiassllc/hitomado/internal/gmail/googleerr"
)

// 利用者が自分で直せる原因の判定に使う Google の reason。errors[].reason（旧形式）と details[].reason（ErrorInfo）の両方を見る。
var (
	apiDisabledReasons       = []string{"SERVICE_DISABLED", "accessNotConfigured"}
	scopeInsufficientReasons = []string{"ACCESS_TOKEN_SCOPE_INSUFFICIENT", "insufficientPermissions"}
)

// APIError は Gmail API が返したエラー応答から取り出した情報。公開 sentinel（ErrNeedsReauth／ErrGeneric など）を
// errors.Is で判定できる。Message は伏せ字処理済みで、トークンやメールアドレスは含まない。
type APIError struct {
	googleerr.Info        // Status・State・Reasons・Message・Metadata
	Op             string // 操作名（getProfile など）
	Kind           error  // 分類済みの sentinel（errors.Is で判定できる）
}

func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "gmail: %s (HTTP %d): %v", e.Op, e.Status, e.Kind)
	var parts []string
	if e.State != "" {
		parts = append(parts, "status="+e.State)
	}
	if len(e.Reasons) > 0 {
		parts = append(parts, "reason="+strings.Join(e.Reasons, ","))
	}
	if e.Message != "" {
		parts = append(parts, fmt.Sprintf("message=%q", e.Message))
	}
	if len(parts) > 0 {
		b.WriteString(" (" + strings.Join(parts, ", ") + ")")
	}
	return b.String()
}

// Unwrap は分類済みの sentinel を返す。
func (e *APIError) Unwrap() error { return e.Kind }

// Is は ErrAPIDisabled／ErrScopeInsufficient を reason から判定する。
func (e *APIError) Is(target error) bool {
	switch target {
	case ErrAPIDisabled:
		return e.hasReason(apiDisabledReasons)
	case ErrScopeInsufficient:
		return e.hasReason(scopeInsufficientReasons)
	}
	return false
}

func (e *APIError) hasReason(want []string) bool {
	for _, r := range e.Reasons {
		for _, w := range want {
			if r == w {
				return true
			}
		}
	}
	return false
}

// logAttrs はログ用の属性。値は構築時に伏せ字処理・文字種の制限を済ませてある。
func (e *APIError) logAttrs() []any {
	attrs := []any{slog.String("op", e.Op), slog.Int("status", e.Status)}
	if e.State != "" {
		attrs = append(attrs, slog.String("api_status", e.State))
	}
	if len(e.Reasons) > 0 {
		attrs = append(attrs, slog.String("reasons", strings.Join(e.Reasons, ",")))
	}
	if e.Message != "" {
		attrs = append(attrs, slog.String("api_message", e.Message))
	}
	if len(e.Metadata) > 0 {
		keys := make([]string, 0, len(e.Metadata))
		for k := range e.Metadata {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([]string, len(keys))
		for i, k := range keys {
			pairs[i] = k + "=" + e.Metadata[k]
		}
		attrs = append(attrs, slog.String("api_metadata", strings.Join(pairs, " ")))
	}
	return attrs
}

// AsAPIError は err の連鎖から Gmail API のエラー応答の情報を取り出す。
func AsAPIError(err error) (*APIError, bool) {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}
