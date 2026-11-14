package dto

// InboxZeroReasons は Inbox Zero（#314）で「不要なメール」とみなす理由ごとのオン・オフ。オンの理由だけを使う。
type InboxZeroReasons struct {
	// Ad は広告・宣伝のメール（機能ごとに使う AI の「Inbox Zero」に選んだ AI が判定）。
	Ad bool `json:"ad"`
	// Notification は通知だけのメール（選んだ AI が判定）。
	Notification bool `json:"notification"`
	// Newsletter は未読のニュースレター（選んだ AI が判定。スレッドが未読のときだけ）。
	Newsletter bool `json:"newsletter"`
	// Replied は返信済みのメール（スレッドの最後の 1 通（下書きを除く）が自分の送信。端末内で判定し、AI は使わない）。
	Replied bool `json:"replied"`
	// Calendar はカレンダーの招待・通知のメール（最後の 1 通（下書きを除く）が iCalendar のパートを持つ、または Google カレンダーからのもの。
	// 端末内で判定し、AI は使わない。#377）。
	Calendar bool `json:"calendar"`
}

// InboxZeroSettings は Inbox Zero の設定（app_settings の inbox_zero.reasons）。未設定・壊れた値は全部オン。
type InboxZeroSettings struct {
	Reasons InboxZeroReasons `json:"reasons"`
}

// InboxZeroReason の値（InboxZeroCandidate.Reason）。
const (
	InboxZeroReasonAd           = "ad"
	InboxZeroReasonNotification = "notification"
	InboxZeroReasonNewsletter   = "newsletter"
	InboxZeroReasonReplied      = "replied"
	InboxZeroReasonCalendar     = "calendar"
)

// InboxZeroCandidate はアーカイブの候補 1 件（FindInboxZeroCandidates の結果）。
type InboxZeroCandidate struct {
	Ref ThreadRef `json:"ref"`
	// From はスレッドの最新のメッセージ（下書きを除く）の差出人（表示名。無ければメールアドレス）。
	From    string `json:"from"`
	Subject string `json:"subject"`
	// Reason は候補にした理由（ad／notification／newsletter／replied／calendar）。
	Reason string `json:"reason"`
}

// InboxZeroResult は FindInboxZeroCandidates の結果。
type InboxZeroResult struct {
	// Candidates は候補（渡された順。null にしない）。
	Candidates []InboxZeroCandidate `json:"candidates"`
	// Examined は調べたスレッドの数（重複を除く）。
	Examined int `json:"examined"`
	// Skipped は AI の判定に失敗して候補にしなかったスレッドの数。
	Skipped int `json:"skipped"`
}
