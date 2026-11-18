package store

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
)

// NotifyScope は新着通知の対象にするメール（設定「通知するメール」。全アカウント共通。#395）。
type NotifyScope string

const (
	// NotifyScopePrimary は受信トレイの「メイン」のメールだけ（既定）。Gmail のカテゴリ
	// （ソーシャル・プロモーション・新着情報・フォーラム）のラベルが付いたメールは通知しない。
	NotifyScopePrimary NotifyScope = "primary"
	// NotifyScopeAll は受信トレイの新着すべて。
	NotifyScopeAll NotifyScope = "all"
)

// notifyCategoryLabels は「メイン」ではないことを示す Gmail のカテゴリのラベル。
var notifyCategoryLabels = []string{"CATEGORY_SOCIAL", "CATEGORY_PROMOTIONS", "CATEGORY_UPDATES", "CATEGORY_FORUMS"}

// Valid は s が定義済みの値かを返す。
func (s NotifyScope) Valid() bool { return s == NotifyScopePrimary || s == NotifyScopeAll }

// Allows はラベルが labelIDs のメッセージを通知してよいかを返す（受信トレイ・未読・下書きの判定は呼び出し側）。
// メインだけのときは、カテゴリのラベルを 1 つでも持つメッセージを通知しない。
func (s NotifyScope) Allows(labelIDs []string) bool {
	if s == NotifyScopeAll {
		return true
	}
	return !slices.ContainsFunc(labelIDs, func(id string) bool { return slices.Contains(notifyCategoryLabels, id) })
}

// LoadNotifyScope は保存済みの設定を返す。未設定・壊れた値・未知の値は NotifyScopePrimary。
func LoadNotifyScope(ctx context.Context, s Store) (NotifyScope, error) {
	raw, ok, err := s.GetSetting(ctx, SettingNotifyScope)
	if err != nil {
		return NotifyScopePrimary, err
	}
	if !ok {
		return NotifyScopePrimary, nil
	}
	var v NotifyScope
	if json.Unmarshal([]byte(raw), &v) != nil || !v.Valid() {
		return NotifyScopePrimary, nil
	}
	return v, nil
}

// SaveNotifyScope は設定を保存する。未知の値は保存しない。
func SaveNotifyScope(ctx context.Context, s Store, v NotifyScope) error {
	if !v.Valid() {
		return fmt.Errorf("store: invalid notify scope %q", string(v))
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("store: encode notify scope: %w", err)
	}
	return s.SetSetting(ctx, SettingNotifyScope, string(b))
}
