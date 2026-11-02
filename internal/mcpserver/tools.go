package mcpserver

import (
	"context"
	"errors"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/logging"
)

// ToolNames は公開するツールの全部。送信するツールはここに無く、作らない。
var ToolNames = []string{
	"get_status", "get_logs", "list_accounts", "list_threads", "search_threads", "get_thread", "create_reply_draft",
}

const instructions = "Hitomado（複数の Gmail を 1 つの受信箱で扱うデスクトップアプリ）の MCP サーバー。" +
	"状態・ログ・アカウント・メールの読み取りと、返信の下書きの作成ができる。" +
	"メールの送信はできない。create_reply_draft が作る下書きは、利用者がアプリで確認して送る。"

type noInput struct{}

type statusOutput struct {
	StatusInfo
	LastLogError *logEntry `json:"lastLogError,omitempty"`
}

type getLogsInput struct {
	Limit int    `json:"limit,omitempty" jsonschema:"返す最大件数（新しい方から。既定 100、最大 1000）"`
	Level string `json:"level,omitempty" jsonschema:"この水準以上: debug / info / warn / error（既定 info）"`
	Since string `json:"since,omitempty" jsonschema:"この時刻以降（RFC 3339。例 2026-11-03T09:00:00+09:00）"`
	Until string `json:"until,omitempty" jsonschema:"この時刻以前（RFC 3339）"`
}

type logEntry struct {
	Time    string         `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Attrs   map[string]any `json:"attrs,omitempty"`
}

type getLogsOutput struct {
	Entries []logEntry `json:"entries"`
}

type listAccountsOutput struct {
	Accounts []AccountInfo `json:"accounts"`
}

type listThreadsInput struct {
	AccountIDs []string `json:"accountIds,omitempty" jsonschema:"対象アカウント ID（省略で全部）"`
	UnreadOnly bool     `json:"unreadOnly,omitempty" jsonschema:"未読のみ"`
	Limit      int      `json:"limit,omitempty" jsonschema:"1〜100（既定 50）"`
	Cursor     string   `json:"cursor,omitempty" jsonschema:"前回の nextCursor"`
}

type searchThreadsInput struct {
	Query      string   `json:"query" jsonschema:"Gmail の検索クエリ（例 from:alice is:unread）"`
	AccountIDs []string `json:"accountIds,omitempty" jsonschema:"対象アカウント ID（省略で全部）"`
	Limit      int      `json:"limit,omitempty" jsonschema:"1〜100（既定 50）"`
	Cursor     string   `json:"cursor,omitempty" jsonschema:"前回の nextCursor"`
}

type getThreadInput struct {
	AccountID string `json:"accountId" jsonschema:"スレッドのあるアカウント ID"`
	ThreadID  string `json:"threadId"`
}

type replyDraftInput struct {
	AccountID       string `json:"accountId" jsonschema:"返信するスレッドを受信したアカウント ID。下書きの送信元はこのアカウントに固定される"`
	ThreadID        string `json:"threadId"`
	SourceMessageID string `json:"sourceMessageId,omitempty" jsonschema:"返信元のメッセージ ID（省略で自分以外の最新）"`
	ReplyAll        bool   `json:"replyAll,omitempty" jsonschema:"全員に返信"`
	Body            string `json:"body" jsonschema:"返信の本文（プレーンテキスト。署名と引用はアプリが付ける）"`
}

// newServer は Backend のツールを持つ MCP サーバーを作る。呼び出しは logger に残す（引数・本文は残さない）。
func newServer(b Backend, logDir string, logger *slog.Logger, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "hitomado", Version: version}, &mcp.ServerOptions{Instructions: instructions})
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: ptr(false)}
	l := &callLogger{logger: logger}
	s.AddReceivingMiddleware(l.middleware)

	addTool(l, s, &mcp.Tool{Name: "get_status", Description: "アプリの版、アカウントごとの同期状態、最後のエラーを返す。", Annotations: readOnly},
		func(ctx context.Context, _ noInput) (statusOutput, error) {
			st, err := b.Status(ctx)
			if err != nil {
				return statusOutput{}, err
			}
			out := statusOutput{StatusInfo: st}
			if es, err := logging.ReadEntries(logDir, logging.Query{Limit: 1, MinLevel: slog.LevelError}); err == nil && len(es) == 1 {
				e := toLogEntry(es[0])
				out.LastLogError = &e
			}
			return out, nil
		})

	addTool(l, s, &mcp.Tool{Name: "get_logs", Description: "アプリのログ・エラーを返す（件数・水準・期間で絞り込み。秘密とメール内容は伏せ済み）。", Annotations: readOnly},
		func(_ context.Context, in getLogsInput) (getLogsOutput, error) {
			q, err := logQuery(in)
			if err != nil {
				return getLogsOutput{}, err
			}
			es, err := logging.ReadEntries(logDir, q)
			if err != nil {
				return getLogsOutput{}, apperr.Wrap(apperr.Internal, "ログを読めません。", err)
			}
			out := getLogsOutput{Entries: make([]logEntry, len(es))}
			for i, e := range es {
				out.Entries[i] = toLogEntry(e)
			}
			return out, nil
		})

	addTool(l, s, &mcp.Tool{Name: "list_accounts", Description: "接続済みの Gmail アカウントの一覧。", Annotations: readOnly},
		func(ctx context.Context, _ noInput) (listAccountsOutput, error) {
			a, err := b.ListAccounts(ctx)
			return listAccountsOutput{Accounts: a}, err
		})

	addTool(l, s, &mcp.Tool{Name: "list_threads", Description: "統合受信箱のスレッド一覧（新しい順）。", Annotations: readOnly},
		func(ctx context.Context, in listThreadsInput) (dto.ThreadPage, error) {
			return b.ListThreads(ctx, dto.InboxQuery{AccountIDs: in.AccountIDs, UnreadOnly: in.UnreadOnly, Limit: in.Limit, Cursor: in.Cursor})
		})

	addTool(l, s, &mcp.Tool{Name: "search_threads", Description: "Gmail の検索クエリでスレッドを探す。", Annotations: readOnly},
		func(ctx context.Context, in searchThreadsInput) (dto.ThreadPage, error) {
			return b.SearchThreads(ctx, dto.SearchQuery{Q: in.Query, AccountIDs: in.AccountIDs, Limit: in.Limit, Cursor: in.Cursor})
		})

	addTool(l, s, &mcp.Tool{Name: "get_thread", Description: "スレッドのメッセージを本文つきで取得する。添付は名前・種類・サイズだけで、中身は返さない。状態（既読など）は変えない。", Annotations: readOnly},
		func(ctx context.Context, in getThreadInput) (ThreadView, error) {
			return b.GetThread(ctx, in.AccountID, in.ThreadID)
		})

	addTool(l, s, &mcp.Tool{
		Name:        "create_reply_draft",
		Description: "スレッドへの返信の下書きを Hitomado の下書き（Gmail の下書き）として作る。送信はしない。送信元は受信したアカウント、宛先・件名・署名・引用はアプリの規則で決まる。利用者がアプリで確認して送る。",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(false), OpenWorldHint: ptr(false)},
	}, func(ctx context.Context, in replyDraftInput) (ReplyDraft, error) {
		return b.CreateReplyDraft(ctx, ReplyDraftRequest{
			AccountID: in.AccountID, ThreadID: in.ThreadID, SourceMessageID: in.SourceMessageID, ReplyAll: in.ReplyAll, BodyText: in.Body,
		})
	})
	return s
}

func ptr[T any](v T) *T { return &v }

func toLogEntry(e logging.Entry) logEntry {
	return logEntry{Time: e.Time.Format(time.RFC3339), Level: e.Level, Message: e.Message, Attrs: e.Attrs}
}

func logQuery(in getLogsInput) (logging.Query, error) {
	lvl, err := logging.ParseLevel(in.Level)
	if err != nil {
		return logging.Query{}, apperr.New(apperr.Validation, "level は debug / info / warn / error のいずれかです。")
	}
	q := logging.Query{Limit: in.Limit, MinLevel: lvl}
	if q.Since, err = parseTime(in.Since); err != nil {
		return logging.Query{}, apperr.New(apperr.Validation, "since は RFC 3339 の時刻で指定してください。")
	}
	if q.Until, err = parseTime(in.Until); err != nil {
		return logging.Query{}, apperr.New(apperr.Validation, "until は RFC 3339 の時刻で指定してください。")
	}
	return q, nil
}

func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

// callLogger は MCP のツール呼び出しを記録する。残すのは時刻（ログの時刻）・ツール名・成否・所要時間・エラーコードだけで、
// 引数（検索クエリ・本文）と結果は残さない。記録は tools/call のミドルウェア 1 か所で行い、ツール本体に届かなかった
// 呼び出し（未知のツール名・不正な引数）も同じ形で残す（エラーコードは PROTOCOL）。
type callLogger struct{ logger *slog.Logger }

// callInfo はミドルウェアとツール本体（addTool）の間でエラーコードを受け渡す。
type callInfo struct{ errCode string }

type callInfoKey struct{}

const (
	codeProtocol      = "PROTOCOL"
	maxToolNameLogged = 64
)

func (c *callLogger) middleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if method != "tools/call" {
			return next(ctx, method, req)
		}
		started := time.Now()
		info := &callInfo{}
		res, err := next(context.WithValue(ctx, callInfoKey{}, info), method, req)
		name := "?"
		if r, ok := req.(*mcp.CallToolRequest); ok && r.Params != nil {
			name = truncate(r.Params.Name, maxToolNameLogged) // 呼び出し側が自由に決める文字列なので切る
		}
		failed := err != nil
		if r, ok := res.(*mcp.CallToolResult); ok && r != nil && r.IsError {
			failed = true
		}
		attrs := []any{"tool", name, "ok", !failed, "duration_ms", time.Since(started).Milliseconds()}
		if !failed {
			c.logger.Info("mcp tool call", attrs...)
			return res, err
		}
		code := info.errCode
		if code == "" {
			code = codeProtocol
		}
		c.logger.Warn("mcp tool call", append(attrs, "error_code", code)...)
		return res, err
	}
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func errorCode(err error) string {
	var ae *apperr.Error
	if errors.As(err, &ae) {
		return ae.Code
	}
	return apperr.Internal
}

// addTool はハンドラを登録する。panic はスタックを記録して内部エラーにし、エラーは apperr に写して（想定外のエラーの文面を
// MCP クライアントへ返さない）、エラーコードを呼び出しの記録（callLogger.middleware）へ渡す。
func addTool[In, Out any](l *callLogger, s *mcp.Server, t *mcp.Tool, h func(context.Context, In) (Out, error)) {
	mcp.AddTool(s, t, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (res *mcp.CallToolResult, out Out, err error) {
		defer func() {
			if r := recover(); r != nil { // ツールの panic で接続ごと落とさない
				logging.LogPanic(l.logger, "mcp:"+t.Name, r, debug.Stack())
				err = apperr.New(apperr.Internal, "内部エラーが発生しました。")
			}
			if info, ok := ctx.Value(callInfoKey{}).(*callInfo); ok && err != nil {
				info.errCode = errorCode(err)
			}
		}()
		out, err = h(ctx, in)
		if err != nil {
			err = apperr.FromInfra(err, "") // 想定外のエラーの文面（パスや本文の断片を含み得る）を MCP クライアントへ返さない
		}
		return nil, out, err
	})
}
