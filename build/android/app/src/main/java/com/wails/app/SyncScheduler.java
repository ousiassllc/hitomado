package com.wails.app;

import android.content.Context;

import androidx.work.Constraints;
import androidx.work.ExistingPeriodicWorkPolicy;
import androidx.work.NetworkType;
import androidx.work.PeriodicWorkRequest;
import androidx.work.WorkManager;

import java.util.concurrent.TimeUnit;

/**
 * 定期同期の登録・解除（WorkManager）。アプリの起動時とアカウントの追加後に Go が {@code scheduleSync} で呼ぶ。
 * 登録は冪等（{@link ExistingPeriodicWorkPolicy#KEEP}）。アカウントが 0 件になったら解除する。
 */
final class SyncScheduler {
    /** ユニーク名。 */
    static final String WORK_NAME = "hitomado-sync";
    /** OS が許す最小の間隔（WorkManager の PeriodicWorkRequest の下限）。 */
    static final long INTERVAL_MINUTES = 15;

    private SyncScheduler() {}

    static PeriodicWorkRequest request() {
        Constraints constraints = new Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build();
        return new PeriodicWorkRequest.Builder(SyncWorker.class, INTERVAL_MINUTES, TimeUnit.MINUTES)
                .setConstraints(constraints)
                .build();
    }

    static void schedule(Context context) {
        WorkManager.getInstance(context.getApplicationContext())
                .enqueueUniquePeriodicWork(WORK_NAME, ExistingPeriodicWorkPolicy.KEEP, request());
    }

    static void cancel(Context context) {
        WorkManager.getInstance(context.getApplicationContext()).cancelUniqueWork(WORK_NAME);
    }
}
