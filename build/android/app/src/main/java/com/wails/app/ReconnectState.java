package com.wails.app;

import android.content.Context;
import android.content.SharedPreferences;

import java.util.HashSet;
import java.util.Set;

/** 「再接続が必要」を通知済みのアカウント ID の集合の保存先（通知を 1 回だけにするため。{@link SyncPlan}）。 */
final class ReconnectState {
    private static final String PREFS = "hitomado_notify_state";
    private static final String KEY = "reconnect_notified";

    private final SharedPreferences prefs;

    ReconnectState(Context context) {
        this.prefs = context.getApplicationContext().getSharedPreferences(PREFS, Context.MODE_PRIVATE);
    }

    synchronized Set<String> load() {
        return new HashSet<>(prefs.getStringSet(KEY, new HashSet<String>())); // 返された集合は変更しない決まりなので複製する
    }

    synchronized void save(Set<String> ids) {
        prefs.edit().putStringSet(KEY, new HashSet<>(ids)).apply();
    }

    synchronized void forget(String accountId) {
        Set<String> ids = load();
        if (ids.remove(accountId)) {
            save(ids);
        }
    }
}
