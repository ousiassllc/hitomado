package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/ousiassllc/hitomado/internal/platform"
	"github.com/ousiassllc/hitomado/internal/store"
)

// loadWindowState は保存されたウィンドウ状態を読む。無い・壊れているときは既定値。
func loadWindowState(ctx context.Context, st store.Store, logger *slog.Logger) platform.WindowState {
	raw, ok, err := st.GetSetting(ctx, store.SettingWindowState)
	if err != nil {
		logger.Warn("load window state failed", "err", err.Error())
		return platform.DefaultWindowState()
	}
	if !ok {
		return platform.DefaultWindowState()
	}
	s, valid := platform.ParseWindowState(raw)
	if !valid {
		return platform.DefaultWindowState()
	}
	return s
}

// windowStateSaver は状態を window.state へ保存する関数を返す。終了処理中でも保存できるよう ctx のキャンセルとは独立に動く。
func windowStateSaver(st store.Store, logger *slog.Logger) func(platform.WindowState) {
	return func(s platform.WindowState) {
		raw, err := s.Encode()
		if err != nil {
			logger.Warn("encode window state failed", "err", err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := st.SetSetting(ctx, store.SettingWindowState, raw); err != nil {
			logger.Warn("save window state failed", "err", err.Error())
		}
	}
}
