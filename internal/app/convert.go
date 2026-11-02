package app

import (
	"context"
	"log/slog"
	"strings"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/store"
)

// accountDTO は store のアカウントをバインディング DTO に変換する。
func accountDTO(a store.Account) dto.Account {
	return dto.Account{
		ID: a.ID, OAuthClientID: a.OAuthClientID, Email: a.Email, DisplayName: a.DisplayName, Color: a.Color,
		SignatureHTML: a.SignatureHTML, NotifyEnabled: a.NotifyEnabled, Status: a.Status,
		StatusMessage: a.StatusMessage, SortOrder: a.SortOrder,
	}
}

func accountsDTO(list []store.Account) []dto.Account {
	out := make([]dto.Account, len(list))
	for i, a := range list {
		out[i] = accountDTO(a)
	}
	return out
}

func labelDTO(l store.Label) dto.Label {
	return dto.Label{ID: l.ID, Name: l.Name, ColorBg: l.ColorBg, ColorFg: l.ColorFg}
}

// userLabelsDTO はユーザー作成ラベルだけを DTO にする（nil を返さない）。
func userLabelsDTO(list []store.Label) []dto.Label {
	out := make([]dto.Label, 0, len(list))
	for _, l := range list {
		if l.Type == "user" {
			out = append(out, labelDTO(l))
		}
	}
	return out
}

// summaryFromRow は DB のスレッド行をサマリ DTO にする。
func summaryFromRow(r store.ThreadRow) dto.ThreadSummary {
	labels := make([]dto.Label, len(r.Labels))
	for i, l := range r.Labels {
		labels[i] = labelDTO(l)
	}
	return dto.ThreadSummary{
		AccountID: r.AccountID, ThreadID: r.ThreadID, Subject: r.Subject, Snippet: r.Snippet,
		LastMessageAt: r.LastMessageAt.UnixMilli(), LastFrom: r.LastFrom, MessageCount: r.MessageCount,
		IsUnread: r.IsUnread, HasAttachment: r.HasAttachment, InInbox: r.InInbox, Labels: labels,
	}
}

func validation(msg string) *apperr.Error { return apperr.New(apperr.Validation, msg) }

func logger(d *Deps) *slog.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	return slog.Default()
}

// gmailErr は Gmail（とその周辺）のエラーを AppError にする。NEEDS_REAUTH はアカウントを要再接続にして
// account:status を発行する（バインディング仕様 §10）。err は nil でないこと。
func gmailErr(d *Deps, ctx context.Context, accountID string, err error) *apperr.Error {
	ae := apperr.FromInfra(err, accountID)
	if ae.Code == apperr.NeedsReauth && accountID != "" {
		markNeedsReauth(d, ctx, accountID, ae.Message)
	}
	return ae
}

func markNeedsReauth(d *Deps, ctx context.Context, accountID, message string) {
	ctx = context.WithoutCancel(ctx)
	acc, err := d.Store.GetAccount(ctx, accountID)
	if err != nil || acc.Status == store.StatusNeedsReauth {
		return
	}
	if err := d.Store.SetAccountStatus(ctx, accountID, store.StatusNeedsReauth, message); err != nil {
		logger(d).Warn("アカウント状態の更新に失敗", "accountId", accountID, "err", err)
		return
	}
	d.Events.Emit(events.AccountStatus, events.AccountStatusPayload{AccountID: accountID, Status: store.StatusNeedsReauth, Message: message})
}

// accountsByID は全アカウントを ID で引ける形で返す。
func accountsByID(d *Deps, ctx context.Context) (list []store.Account, byID map[string]store.Account, err error) {
	list, err = d.Store.ListAccounts(ctx)
	if err != nil {
		return nil, nil, apperr.FromInfra(err, "")
	}
	byID = make(map[string]store.Account, len(list))
	for _, a := range list {
		byID[a.ID] = a
	}
	return list, byID, nil
}

// targetAccounts は ids（空は全アカウント）に該当する既存アカウントを一覧の順で返す。未知の ID は無視する。
func targetAccounts(list []store.Account, ids []string) []store.Account {
	if len(ids) == 0 {
		return list
	}
	want := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		want[id] = struct{}{}
	}
	out := make([]store.Account, 0, len(ids))
	for _, a := range list {
		if _, ok := want[a.ID]; ok {
			out = append(out, a)
		}
	}
	return out
}

func trimmed(s string) string { return strings.TrimSpace(s) }
