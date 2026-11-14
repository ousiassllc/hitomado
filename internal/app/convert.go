package app

import (
	"context"
	"log/slog"
	"strings"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
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

func logger(d *Deps) *slog.Logger { return (*deps.Deps)(d).Log() }

// gmailErr は Gmail（とその周辺）のエラーを AppError にする。NEEDS_REAUTH はアカウントを要再接続にして
// account:status を発行する（バインディング仕様 §10）。err は nil でないこと。
func gmailErr(d *Deps, ctx context.Context, accountID string, err error) *apperr.Error {
	ae := apperr.FromInfra(err, accountID)
	if ae.Code == apperr.NeedsReauth && accountID != "" {
		message := ae.Message
		if gmail.ReauthReason(err) == gmail.ReasonInvalidGrant {
			message = syncer.MsgTokenRevoked // 同期周期の経路と同じメッセージにして、画面が原因の案内を出せるようにする
		}
		(*deps.Deps)(d).MarkNeedsReauth(ctx, accountID, message, err)
	}
	return ae
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

// threadGone は操作対象のスレッドが Gmail に無い（404）ときのエラー。
func threadGone(accountID string) *apperr.Error {
	return apperr.New(apperr.NotFound, "スレッドが見つかりません。").ForAccount(accountID)
}

// forgetAccounts は gmail.Client の任意メソッド Forget（認証情報キャッシュの破棄）があれば各アカウントに呼ぶ。
func forgetAccounts(d *Deps, accountIDs ...string) {
	f, ok := d.Gmail.(tokenForgetter)
	if !ok {
		return
	}
	for _, id := range accountIDs {
		f.Forget(id)
	}
}
