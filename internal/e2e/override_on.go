//go:build e2e && !production

package e2e

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/store"
)

// Marker は E2E モードのバイナリにだけ入る文字列。リリース APK の検査（build/android/scripts/verify-apk.sh）が、
// libwails.so にこの文字列が無いことを確かめる。
const Marker = "hitomado-e2e-backend-v1"

// Enabled は E2E 用ビルドのとき true。
const Enabled = true

// AgentMascotEnv が "1" のとき、エージェントモードとマスコットをオンにした状態で起動する。Windows の CI（.github/scripts/test-mascot.ps1）が、
// マスコットの別ウィンドウがメインウィンドウを閉じても残ることを確かめるための入口（AI は未設定のまま。新着の処理は動かない）。
const AgentMascotEnv = "HITOMADO_E2E_AGENT_MASCOT"

// Override は渡された secrets・gmail を捨て、メモリ上の資格情報と、シード済みのフェイク Gmail に差し替える。
// 初回（アカウントが無いとき）は OAuth クライアントとアカウントを 1 件ずつ DB に作る。
// 同期は通常どおりこのフェイクに対して走り、スレッド一覧が DB に入る。
func Override(ctx context.Context, dir string, st store.Store, _ secrets.Store, _ gmail.Client, logger *slog.Logger) (secrets.Store, gmail.Client, error) {
	sec := secrets.NewMemory()
	fake, err := Seed(ctx, st, sec, timeNow())
	if err != nil {
		return nil, nil, err
	}
	if os.Getenv(AgentMascotEnv) == "1" {
		if err := st.SetSetting(ctx, store.SettingAgent, `{"enabled":true,"mascotEnabled":true}`); err != nil {
			return nil, nil, fmt.Errorf("e2e: seed agent settings: %w", err)
		}
		// マスコットに件数が出るよう、確認待ちと質問待ちを 1 件ずつ置く（再起動しても同じ 2 件のまま）。
		for i, status := range []string{store.AgentStatusReady, store.AgentStatusAsking} {
			it := store.AgentItem{
				AccountID: AccountID, ThreadID: fmt.Sprintf("e2e-thread-%d", i+1), MessageID: fmt.Sprintf("e2e-agent-msg-%d", i+1), Status: status,
				Subject: "E2E agent item", FromLabel: "E2E sender", ReasonKind: "needs_answer", Reason: "E2E",
			}
			if status == store.AgentStatusAsking {
				it.Questions = []string{"E2E question"}
			}
			if _, err := st.CreateAgentItem(ctx, it); err != nil && !errors.Is(err, store.ErrDuplicate) {
				return nil, nil, fmt.Errorf("e2e: seed agent item: %w", err)
			}
		}
	}
	logger.Warn("E2E mode: Gmail is a seeded fake and credentials are in memory; never use with real accounts", "marker", Marker)
	return sec, NewRecordingGmail(fake, dir), nil
}
