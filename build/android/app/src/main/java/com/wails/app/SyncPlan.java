package com.wails.app;

import java.util.ArrayList;
import java.util.Collections;
import java.util.HashSet;
import java.util.List;
import java.util.Set;

/**
 * 同期の結果（{@link SyncOutcome}）から、Worker が何をするか（通知・戻り値）を決める純 Java。JVM で単体テストする。
 *
 * <ul>
 *   <li>新着はそのまま通知する（アカウントの通知設定は Go が反映済み）。
 *   <li>再接続が必要（needs_reauth）なアカウントは、「要再接続」の通知を<b>1 回だけ</b>出す。出した記録は、そのアカウントが
 *       再び同期できる（ok）か、アカウントが無くなるまで残す。
 *   <li>戻り値: 一時的な失敗（失敗の JSON、または同期できたアカウントが無く一時エラーだけ）は {@link Result#RETRY}（WorkManager の
 *       バックオフで再試行）。ただし試行が {@link #MAX_ATTEMPTS} 回に達したら SUCCESS（次の定期実行に任せる）。
 *       再接続が必要なだけの結果は再試行しても直らないので SUCCESS。別の同期が実行中（busy）も SUCCESS。
 * </ul>
 */
final class SyncPlan {
    enum Result { SUCCESS, RETRY }

    /** これ以上は retry せず、次の定期実行に任せる試行回数（WorkManager の runAttemptCount は 0 始まり）。 */
    static final int MAX_ATTEMPTS = 3;

    final Result result;
    final List<SyncOutcome.NewMail> notify;
    /** 「再接続が必要」の通知を出すアカウント。 */
    final List<SyncOutcome.Account> reconnect;
    /** 実行後の「再接続の通知を出した」アカウント ID の集合（保存する）。 */
    final Set<String> reconnectNotified;

    private SyncPlan(Result result, List<SyncOutcome.NewMail> notify, List<SyncOutcome.Account> reconnect, Set<String> reconnectNotified) {
        this.result = result;
        this.notify = Collections.unmodifiableList(notify);
        this.reconnect = Collections.unmodifiableList(reconnect);
        this.reconnectNotified = Collections.unmodifiableSet(reconnectNotified);
    }

    /**
     * @param runAttemptCount WorkManager の Worker#getRunAttemptCount（初回は 0）
     * @param alreadyNotified これまでに「再接続が必要」を通知したアカウント ID
     */
    static SyncPlan of(SyncOutcome o, int runAttemptCount, Set<String> alreadyNotified) {
        boolean canRetry = runAttemptCount + 1 < MAX_ATTEMPTS;
        Set<String> notified = new HashSet<>(alreadyNotified);
        List<SyncOutcome.Account> reconnect = new ArrayList<>();

        if (!o.ok) {
            Result r = SyncOutcome.ERROR_BUSY.equals(o.error) || !canRetry ? Result.SUCCESS : Result.RETRY;
            return new SyncPlan(r, new ArrayList<SyncOutcome.NewMail>(), reconnect, notified);
        }

        boolean anyOk = false;
        boolean anyTransient = false;
        Set<String> present = new HashSet<>();
        for (SyncOutcome.Account a : o.accounts) {
            present.add(a.id);
            if (SyncOutcome.STATUS_NEEDS_REAUTH.equals(a.status)) {
                if (notified.add(a.id)) {
                    reconnect.add(a);
                }
            } else if (SyncOutcome.STATUS_OK.equals(a.status)) {
                anyOk = true;
                notified.remove(a.id);
            } else {
                anyTransient = true;
            }
        }
        // アカウントが無くなった（削除された）ものの記録は消す。ただし appRunning（アプリ本体が同期した）の結果は
        // アカウント一覧が空のことがあるため、記録を消さない。
        if (!o.appRunning) {
            notified.retainAll(present);
        }

        Result r = anyTransient && !anyOk && canRetry ? Result.RETRY : Result.SUCCESS;
        return new SyncPlan(r, new ArrayList<>(o.newMail), reconnect, notified);
    }
}
