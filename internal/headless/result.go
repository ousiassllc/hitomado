// Package headless は Wails に依存しない「1 回だけ同期して新着を返す」入口を提供する（Android の WorkManager の Worker 用。#88）。
//
// 組み立てるのは store・gmail・sync だけで、ウィンドウ・トレイ・通知サービス・イベントには触れない。
// 新着の通知は呼び出し側（Java の NotificationManager）が出すため、同期が見つけた新着を集めて返す。
package headless

import (
	"errors"

	"github.com/ousiassllc/hitomado/internal/asciijson"
	"github.com/ousiassllc/hitomado/internal/notify"
)

// AccountStatus はアカウント 1 件の今回の同期の結果。
type AccountStatus string

const (
	// StatusOK は同期できた。
	StatusOK AccountStatus = "ok"
	// StatusNeedsReauth は再接続が必要（リフレッシュトークンの失効・喪失、Client ID／Secret の拒否）。再試行しても直らない。
	StatusNeedsReauth AccountStatus = "needs_reauth"
	// StatusError は一時的な失敗（オフライン・制限中など）。次の周期で再試行する。
	StatusError AccountStatus = "error"
)

// AccountResult はアカウント 1 件の同期結果。
type AccountResult struct {
	ID     string
	Email  string
	Status AccountStatus
}

// Result は 1 回の同期の結果。
type Result struct {
	// NewMail は今回の同期で見つけた新着（1 スレッド 1 件。通知が有効なアカウントの INBOX の未読だけ）。
	// AppRunning のときは空（アプリ本体の同期が通知を出す）。
	NewMail []notify.NewMailNotice
	// Accounts は同期の対象にした（または再接続待ちの）アカウントごとの結果。
	Accounts []AccountResult
	// AppRunning は、アプリ本体（前面の 60 秒ループ）がこのプロセスで動いていたため、それに同期させたことを表す。
	AppRunning bool
}

// ErrBusy は別のヘッドレス同期が実行中であることを表す。
var ErrBusy = errors.New("headless: another sync is running")

// Java へ返す JSON の失敗コード（SyncOutcome.java と一致させる）。
const (
	codeBusy   = "busy"
	codeFailed = "failed"
)

// jsonMail の表示文字列（title・body）は、Windows のトーストと同じ規則で整える（notify.Text。制御文字・不可視文字の除去と
// 長さの切り詰め。F-10）。アカウントのアドレスは通知チャネルの表示名に使う。
type jsonMail struct {
	AccountID    string `json:"accountId"`
	AccountEmail string `json:"accountEmail"`
	ThreadID     string `json:"threadId"`
	Title        string `json:"title"`
	Body         string `json:"body"`
}

type jsonAccount struct {
	ID     string        `json:"id"`
	Email  string        `json:"email"`
	Status AccountStatus `json:"status"`
}

type jsonResult struct {
	OK         bool          `json:"ok"`
	Error      string        `json:"error,omitempty"`
	AppRunning bool          `json:"appRunning,omitempty"`
	NewMail    []jsonMail    `json:"newMail"`
	Accounts   []jsonAccount `json:"accounts"`
}

// Encode は Java（SyncOutcome.parse）へ返す JSON にする。ASCII だけにする（JNI の制約。asciijson）。
// 失敗（err が非 nil）は {"ok":false,"error":"busy|failed"} だけで、エラーの詳細は載せない（件名・宛先などを含み得るため）。
func Encode(r Result, err error) string {
	out := jsonResult{OK: err == nil, AppRunning: r.AppRunning, NewMail: []jsonMail{}, Accounts: []jsonAccount{}}
	if err != nil {
		out.Error = codeFailed
		if errors.Is(err, ErrBusy) {
			out.Error = codeBusy
		}
	}
	for _, n := range r.NewMail {
		title, body := notify.Text(n)
		out.NewMail = append(out.NewMail, jsonMail{
			AccountID: n.AccountID, AccountEmail: notify.PlainText(n.AccountEmail), ThreadID: n.ThreadID, Title: title, Body: body,
		})
	}
	for _, a := range r.Accounts {
		out.Accounts = append(out.Accounts, jsonAccount{ID: a.ID, Email: a.Email, Status: a.Status})
	}
	s, mErr := asciijson.Marshal(out)
	if mErr != nil {
		return `{"ok":false,"error":"failed","newMail":[],"accounts":[]}`
	}
	return s
}
