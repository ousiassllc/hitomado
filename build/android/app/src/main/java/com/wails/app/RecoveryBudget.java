package com.wails.app;

import java.util.ArrayDeque;

/**
 * 「直近の windowMillis の間に maxAttempts 回まで」の再試行の上限（WebView の復旧・初回読み込みの再試行。#90）。
 * 上限を超えたら false を返し、呼び出し側は再試行をやめてエラー画面を出す（無限ループの防止）。
 * Android に依存せず、時刻は呼び出し側が渡す（JVM で単体テストする）。UI スレッドからだけ使う。
 */
final class RecoveryBudget {
    private final int maxAttempts;
    private final long windowMillis;
    private final ArrayDeque<Long> attempts = new ArrayDeque<>();

    RecoveryBudget(int maxAttempts, long windowMillis) {
        if (maxAttempts < 1 || windowMillis < 1) {
            throw new IllegalArgumentException("maxAttempts and windowMillis must be positive");
        }
        this.maxAttempts = maxAttempts;
        this.windowMillis = windowMillis;
    }

    /** nowMillis に 1 回試行してよければ記録して true。窓の中の試行がすでに上限なら記録せず false。 */
    boolean tryAcquire(long nowMillis) {
        while (!attempts.isEmpty() && nowMillis - attempts.peekFirst() >= windowMillis) {
            attempts.removeFirst();
        }
        if (attempts.size() >= maxAttempts) {
            return false;
        }
        attempts.addLast(nowMillis);
        return true;
    }
}
