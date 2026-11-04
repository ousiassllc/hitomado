package com.wails.app;

import android.content.Context;
import android.util.Log;

import androidx.annotation.NonNull;
import androidx.work.Worker;
import androidx.work.WorkerParameters;

import java.util.Set;

/**
 * 定期同期（WorkManager。15 分間隔・ネットワーク接続が条件）の Worker。Go のヘッドレス同期（{@link HeadlessSync}）を 1 回呼び、
 * 見つかった新着を {@link NewMailNotifier} で通知する。何をするか・戻り値は {@link SyncPlan}。
 *
 * <p>アプリが前面にいる間は Go の 60 秒ループが同期している。そのプロセスでこの Worker が動いたときは、Go 側（Gate）が
 * アプリ本体のスケジューラに「今すぐ 1 回更新」を頼み、新着の通知はアプリ本体の通知経路が出す（結果の newMail は空）。
 */
public final class SyncWorker extends Worker {
    private static final String TAG = "SyncWorker";

    public SyncWorker(@NonNull Context context, @NonNull WorkerParameters params) {
        super(context, params);
    }

    @NonNull
    @Override
    public Result doWork() {
        Context context = getApplicationContext();
        SyncOutcome outcome = HeadlessSync.run(context);
        ReconnectState state = new ReconnectState(context);
        SyncPlan plan = SyncPlan.of(outcome, getRunAttemptCount(), state.load());

        NewMailNotifier notifier = new NewMailNotifier(context);
        for (SyncOutcome.NewMail m : plan.notify) {
            notifier.postNewMail(m.accountId, m.accountEmail, m.threadId, m.title, m.body);
        }
        for (SyncOutcome.Account a : plan.reconnect) {
            notifier.postReconnect(a.id, a.email);
        }
        Set<String> saved = plan.reconnectNotified;
        state.save(saved);
        if (BuildConfig.DEBUG) {
            Log.d(TAG, "sync done: ok=" + outcome.ok + " new=" + plan.notify.size() + " result=" + plan.result);
        }
        return plan.result == SyncPlan.Result.RETRY ? Result.retry() : Result.success();
    }

    @Override
    public void onStopped() {
        super.onStopped();
        HeadlessSync.cancel(); // OS が Worker を止める（制約が外れた・時間切れ）。Go の同期を取り消す
    }
}
