package com.wails.app;

import android.content.Intent;

/**
 * 通知のタップで開くスレッド（アカウント ID・スレッド ID）を 1 件だけ保持する、プロセス内の受け渡し口。
 *
 * <p>MainActivity の onCreate／onNewIntent が Intent の extras から {@link #set} し、Go が起動したあと（または起動済みなら
 * 合図のイベントを受けて）{@link #take} で取り出す。Go（Wails）が起動する前に Java が発行したイベントは取りこぼされるため、
 * 要求そのものは Java が保持して Go が取りに来る。取り出すと空になる（二重に開かない）。最新の 1 件だけを保持する。
 */
final class OpenRequest {
    private static OpenRequest pending;

    final String accountId;
    final String threadId;

    OpenRequest(String accountId, String threadId) {
        this.accountId = accountId;
        this.threadId = threadId;
    }

    /** Intent が通知のタップ（アクションが ACTION_OPEN_THREAD で、ID が両方ある）なら要求にする。違えば null。 */
    static OpenRequest fromIntent(Intent intent) {
        if (intent == null || !NotificationPlan.ACTION_OPEN_THREAD.equals(intent.getAction())) {
            return null;
        }
        return of(intent.getStringExtra(NotificationPlan.EXTRA_ACCOUNT_ID), intent.getStringExtra(NotificationPlan.EXTRA_THREAD_ID));
    }

    static OpenRequest of(String accountId, String threadId) {
        if (accountId == null || accountId.isEmpty() || threadId == null || threadId.isEmpty()) {
            return null;
        }
        return new OpenRequest(accountId, threadId);
    }

    static synchronized void set(OpenRequest r) {
        if (r != null) {
            pending = r;
        }
    }

    /** 保持している要求を取り出す（無ければ null）。取り出すと空になる。 */
    static synchronized OpenRequest take() {
        OpenRequest r = pending;
        pending = null;
        return r;
    }

    /** Go（takeOpenRequest）へ返す JSON。{"accountId":"…","threadId":"…"}。ID は ASCII（ULID・Gmail のスレッド ID）。 */
    String toJson() {
        return "{\"accountId\":" + quote(accountId) + ",\"threadId\":" + quote(threadId) + "}";
    }

    private static String quote(String s) {
        StringBuilder b = new StringBuilder("\"");
        for (int i = 0; i < s.length(); i++) {
            char c = s.charAt(i);
            if (c == '"' || c == '\\') {
                b.append('\\').append(c);
            } else if (c < 0x20 || c > 0x7e) {
                b.append(String.format("\\u%04x", (int) c));
            } else {
                b.append(c);
            }
        }
        return b.append('"').toString();
    }
}
