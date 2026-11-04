package com.wails.app;

/**
 * 通知の「名前・ID・Intent の extras」を決める純 Java（Android に依存しない。JVM で単体テストする）。
 *
 * <ul>
 *   <li>通知チャネルはアカウントごと。チャネル ID は {@code mail_<アカウント ID>}、表示名はメールアドレス
 *       （OS の通知設定の画面でアカウントごとに音・重要度を変えられる）。アカウント削除でチャネルも削除する。
 *   <li>グループは 1 アカウント 1 グループ（グループキーはアカウント ID ベース）。スレッドごとに 1 通知、
 *       アカウントごとにサマリー通知を 1 つ。
 *   <li>通知のタップで MainActivity を開く Intent の extras はアカウント ID とスレッド ID だけ（件名・差出人は載せない）。
 * </ul>
 */
final class NotificationPlan {
    private NotificationPlan() {}

    static final String CHANNEL_PREFIX = "mail_";
    static final String GROUP_PREFIX = "account_";
    /** 「再接続が必要」の通知のチャネル（アカウントによらず 1 つ）。 */
    static final String RECONNECT_CHANNEL_ID = "reconnect";
    /** 通知タップの Intent の extras（MainActivity が読む）。 */
    static final String EXTRA_ACCOUNT_ID = "hitomado.accountId";
    static final String EXTRA_THREAD_ID = "hitomado.threadId";
    /** 通知タップの Intent のアクション。 */
    static final String ACTION_OPEN_THREAD = "jp.ousias.hitomado.OPEN_THREAD";

    /** 通知の表示名の上限（チャネル名は OS が長すぎる名前を切るが、見た目のため 80 字に収める）。 */
    private static final int MAX_CHANNEL_NAME = 80;

    static String channelId(String accountId) {
        return CHANNEL_PREFIX + accountId;
    }

    /** チャネル ID（mail_…）なら対応するアカウント ID、そうでなければ null。 */
    static String accountIdOfChannel(String channelId) {
        if (channelId != null && channelId.startsWith(CHANNEL_PREFIX) && channelId.length() > CHANNEL_PREFIX.length()) {
            return channelId.substring(CHANNEL_PREFIX.length());
        }
        return null;
    }

    static String groupKey(String accountId) {
        return GROUP_PREFIX + accountId;
    }

    /** チャネルの表示名はメールアドレス。空のときはアカウント ID（空の名前のチャネルを作れないため）。 */
    static String channelName(String email, String accountId) {
        String name = email == null ? "" : email.trim();
        if (name.isEmpty()) {
            name = accountId;
        }
        if (name.length() > MAX_CHANNEL_NAME) {
            name = name.substring(0, MAX_CHANNEL_NAME);
        }
        return name;
    }

    /** スレッド 1 件の通知 ID。同じスレッドの新着は同じ ID で更新される（スレッドごとに 1 通知）。 */
    static int threadNotificationId(String accountId, String threadId) {
        return mix(("t\u0000" + accountId + "\u0000" + threadId).hashCode());
    }

    /** アカウントのサマリー通知の ID。 */
    static int summaryNotificationId(String accountId) {
        return mix(("s\u0000" + accountId).hashCode());
    }

    /** 「再接続が必要」の通知の ID（アカウントごとに 1 つ。同じ ID なら重ねて出ない）。 */
    static int reconnectNotificationId(String accountId) {
        return mix(("r\u0000" + accountId).hashCode());
    }

    /** PendingIntent の requestCode（extras だけが違う Intent を別物として扱わせる）。通知 ID と同じ値でよい。 */
    static int requestCode(String accountId, String threadId) {
        return threadNotificationId(accountId, threadId);
    }

    /** 0 を避ける（0 は「通知なし」の意味に使われることがある）。 */
    private static int mix(int h) {
        return h == 0 ? 1 : h;
    }

    /** サマリー通知の本文（件数）。 */
    static String summaryText(int count) {
        return count + " 件の新着メール";
    }

    /** ロック画面（VISIBILITY_PRIVATE）で見せる、内容を含まない通知の本文。 */
    static String publicText() {
        return "新着メールがあります";
    }

    /** 「再接続が必要」の通知の本文。メールアドレス以外の内容を含まない。 */
    static String reconnectText(String email) {
        return email + " の再接続が必要です";
    }
}
