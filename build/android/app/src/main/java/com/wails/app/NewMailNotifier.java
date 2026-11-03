package com.wails.app;

import android.Manifest;
import android.app.Activity;
import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.Build;
import android.provider.Settings;
import android.service.notification.StatusBarNotification;
import android.util.Log;

import androidx.core.app.NotificationCompat;
import androidx.core.app.NotificationManagerCompat;

/**
 * 新着メール・「再接続が必要」の通知を NotificationManager で出す（#88）。applicationContext だけを使い、Activity に依存しない
 * （WorkManager の Worker と、アプリのプロセスが動いているとき（Go の 60 秒ループ。WailsBridge.notifyNewMail）の両方から使う）。
 *
 * <p>名前・ID・extras は {@link NotificationPlan}。ロック画面では内容を隠す（VISIBILITY_PRIVATE。公開版の通知は本文を含まない）。
 * 新着の見出し・本文は Go が Windows のトーストと同じ規則で整形済み（F-10: 見出しはアカウント名、本文は差出人と件名）。
 */
final class NewMailNotifier {
    private static final String TAG = "NewMailNotifier";

    private final Context context;
    private final NotificationManager manager;

    NewMailNotifier(Context context) {
        this.context = context.getApplicationContext();
        this.manager = (NotificationManager) this.context.getSystemService(Context.NOTIFICATION_SERVICE);
    }

    /** 通知を出せるか（Android 13 以降は POST_NOTIFICATIONS の許可、さらに OS のアプリ全体の通知設定がオン）。 */
    boolean canNotify() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU
                && context.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            return false;
        }
        return NotificationManagerCompat.from(context).areNotificationsEnabled();
    }

    /** 許可のダイアログを出す（Android 13 以降で未許可のときだけ。許可済み・永続的に拒否済みなら何も出ない）。 */
    void requestPermission(Activity activity, int requestCode) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU
                && activity.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            activity.requestPermissions(new String[]{Manifest.permission.POST_NOTIFICATIONS}, requestCode);
        }
    }

    /** OS の「このアプリの通知設定」を開く。開けなければ false。 */
    boolean openSettings(Activity activity) {
        try {
            Intent intent = new Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS)
                    .putExtra(Settings.EXTRA_APP_PACKAGE, context.getPackageName());
            activity.startActivity(intent);
            return true;
        } catch (Exception e) {
            try {
                activity.startActivity(new Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS,
                        Uri.fromParts("package", context.getPackageName(), null)));
                return true;
            } catch (Exception e2) {
                Log.e(TAG, "open notification settings failed: " + e2.getClass().getName());
                return false;
            }
        }
    }

    /** アカウントのチャネル（なければ作る）。表示名はメールアドレス。API 26 未満は何もしない。 */
    private void ensureChannel(String accountId, String email) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) {
            return;
        }
        NotificationChannel ch = new NotificationChannel(NotificationPlan.channelId(accountId),
                NotificationPlan.channelName(email, accountId), NotificationManager.IMPORTANCE_DEFAULT);
        ch.setLockscreenVisibility(Notification.VISIBILITY_PRIVATE);
        manager.createNotificationChannel(ch); // 既にあれば何も変えない（利用者が変えた設定を保つ。名前だけは更新される）
    }

    private PendingIntent openThreadIntent(String accountId, String threadId) {
        Intent intent = new Intent(context, MainActivity.class)
                .setAction(NotificationPlan.ACTION_OPEN_THREAD)
                .putExtra(NotificationPlan.EXTRA_ACCOUNT_ID, accountId)
                .putExtra(NotificationPlan.EXTRA_THREAD_ID, threadId)
                .setFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TOP);
        return PendingIntent.getActivity(context, NotificationPlan.requestCode(accountId, threadId), intent,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
    }

    private PendingIntent openAppIntent(String accountId) {
        Intent intent = new Intent(context, MainActivity.class)
                .setFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TOP);
        return PendingIntent.getActivity(context, NotificationPlan.summaryNotificationId(accountId), intent,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
    }

    /** ロック画面に出す内容を含まない版。 */
    private Notification publicVersion(String channelId) {
        return new NotificationCompat.Builder(context, channelId)
                .setSmallIcon(R.drawable.ic_stat_mail)
                .setContentTitle(context.getString(R.string.app_name))
                .setContentText(NotificationPlan.publicText())
                .build();
    }

    /**
     * 新着 1 スレッドを通知する（同じスレッドは同じ通知 ID で更新され、スレッドごとに 1 通知）。アカウントごとのサマリーも更新する。
     * 通知を出せない（許可なし）ときは何もしない。
     */
    void postNewMail(String accountId, String accountEmail, String threadId, String title, String body) {
        if (!canNotify()) {
            return;
        }
        ensureChannel(accountId, accountEmail);
        String channelId = NotificationPlan.channelId(accountId);
        String group = NotificationPlan.groupKey(accountId);
        Notification thread = new NotificationCompat.Builder(context, channelId)
                .setSmallIcon(R.drawable.ic_stat_mail)
                .setContentTitle(title)
                .setContentText(oneLine(body))
                .setStyle(new NotificationCompat.BigTextStyle().bigText(body))
                .setCategory(NotificationCompat.CATEGORY_EMAIL)
                .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
                .setPublicVersion(publicVersion(channelId))
                .setGroup(group)
                .setAutoCancel(true)
                .setContentIntent(openThreadIntent(accountId, threadId))
                .build();
        manager.notify(accountId, NotificationPlan.threadNotificationId(accountId, threadId), thread);
        postSummary(accountId, accountEmail, channelId, group);
    }

    private void postSummary(String accountId, String accountEmail, String channelId, String group) {
        int count = Math.max(1, activeThreadCount(accountId));
        Notification summary = new NotificationCompat.Builder(context, channelId)
                .setSmallIcon(R.drawable.ic_stat_mail)
                .setContentTitle(NotificationPlan.channelName(accountEmail, accountId))
                .setContentText(NotificationPlan.summaryText(count))
                .setStyle(new NotificationCompat.InboxStyle().setSummaryText(NotificationPlan.channelName(accountEmail, accountId)))
                .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
                .setPublicVersion(publicVersion(channelId))
                .setGroup(group)
                .setGroupSummary(true)
                .setAutoCancel(true)
                .setContentIntent(openAppIntent(accountId))
                .build();
        manager.notify(accountId, NotificationPlan.summaryNotificationId(accountId), summary);
    }

    /** 表示中のこのアカウントのスレッド通知の数（サマリーを除く）。 */
    private int activeThreadCount(String accountId) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.M) {
            return 1;
        }
        int summaryId = NotificationPlan.summaryNotificationId(accountId);
        int n = 0;
        for (StatusBarNotification s : manager.getActiveNotifications()) {
            if (accountId.equals(s.getTag()) && s.getId() != summaryId) {
                n++;
            }
        }
        return n;
    }

    /** 「再接続が必要」を通知する（アカウントごとに 1 つ。タップでアプリを開く）。 */
    void postReconnect(String accountId, String email) {
        if (!canNotify()) {
            return;
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            NotificationChannel ch = new NotificationChannel(NotificationPlan.RECONNECT_CHANNEL_ID,
                    "再接続が必要", NotificationManager.IMPORTANCE_DEFAULT);
            ch.setLockscreenVisibility(Notification.VISIBILITY_PRIVATE);
            manager.createNotificationChannel(ch);
        }
        Notification n = new NotificationCompat.Builder(context, NotificationPlan.RECONNECT_CHANNEL_ID)
                .setSmallIcon(R.drawable.ic_stat_mail)
                .setContentTitle(context.getString(R.string.app_name))
                .setContentText(NotificationPlan.reconnectText(email))
                .setCategory(NotificationCompat.CATEGORY_ERROR)
                .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
                .setPublicVersion(publicVersion(NotificationPlan.RECONNECT_CHANNEL_ID))
                .setAutoCancel(true)
                .setContentIntent(openAppIntent(accountId))
                .build();
        manager.notify(accountId, NotificationPlan.reconnectNotificationId(accountId), n);
    }

    /** アカウント削除: 通知チャネルと、そのアカウントの表示中の通知（スレッド・サマリー・再接続）を消す。 */
    void forgetAccount(String accountId) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
            for (StatusBarNotification s : manager.getActiveNotifications()) {
                if (accountId.equals(s.getTag())) {
                    manager.cancel(s.getTag(), s.getId());
                }
            }
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            manager.deleteNotificationChannel(NotificationPlan.channelId(accountId));
        }
    }

    /** 折りたたみ表示の 1 行（差出人と件名の改行を区切りに置き換える）。 */
    private static String oneLine(String body) {
        return body.replace("\n", " — ");
    }
}
