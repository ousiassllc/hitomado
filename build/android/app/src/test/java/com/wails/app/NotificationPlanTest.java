package com.wails.app;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNotEquals;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

/** チャネル・グループ・通知 ID・extras の決め方（NotificationPlan）のテスト。 */
public class NotificationPlanTest {

    @Test
    public void channelIsPerAccountAndNamedByEmail() {
        assertEquals("mail_01HZX", NotificationPlan.channelId("01HZX"));
        assertNotEquals(NotificationPlan.channelId("a"), NotificationPlan.channelId("b"));
        assertEquals("taro@example.com", NotificationPlan.channelName("taro@example.com", "01HZX"));
        assertEquals("taro@example.com", NotificationPlan.channelName("  taro@example.com ", "01HZX"));
    }

    @Test
    public void channelNameFallsBackToAccountIdAndIsTruncated() {
        assertEquals("01HZX", NotificationPlan.channelName("", "01HZX"));
        assertEquals("01HZX", NotificationPlan.channelName(null, "01HZX"));
        StringBuilder longName = new StringBuilder();
        for (int i = 0; i < 200; i++) {
            longName.append('x');
        }
        assertEquals(80, NotificationPlan.channelName(longName.toString(), "a").length());
    }

    @Test
    public void accountIdOfChannelIsTheInverseOfChannelId() {
        assertEquals("01HZX", NotificationPlan.accountIdOfChannel(NotificationPlan.channelId("01HZX")));
        assertNull(NotificationPlan.accountIdOfChannel(NotificationPlan.RECONNECT_CHANNEL_ID));
        assertNull(NotificationPlan.accountIdOfChannel("mail_"));
        assertNull(NotificationPlan.accountIdOfChannel(null));
    }

    @Test
    public void groupKeyIsPerAccount() {
        assertEquals("account_a", NotificationPlan.groupKey("a"));
        assertNotEquals(NotificationPlan.groupKey("a"), NotificationPlan.groupKey("b"));
    }

    @Test
    public void notificationIdsAreStablePerThreadAndDistinctByKind() {
        int t = NotificationPlan.threadNotificationId("a", "t1");
        assertEquals("同じスレッドは同じ ID（1 スレッド 1 通知）", t, NotificationPlan.threadNotificationId("a", "t1"));
        assertNotEquals(t, NotificationPlan.threadNotificationId("a", "t2"));
        assertNotEquals("アカウントが違えば別の通知", t, NotificationPlan.threadNotificationId("b", "t1"));
        assertNotEquals(NotificationPlan.summaryNotificationId("a"), NotificationPlan.summaryNotificationId("b"));
        assertNotEquals(t, NotificationPlan.summaryNotificationId("a"));
        assertNotEquals(NotificationPlan.summaryNotificationId("a"), NotificationPlan.reconnectNotificationId("a"));
        assertNotEquals(0, t);
        assertEquals(t, NotificationPlan.requestCode("a", "t1"));
    }

    @Test
    public void textsDoNotLeakMailContentIntoPublicVersion() {
        // ロック画面（VISIBILITY_PRIVATE）の公開版の本文は固定で、件名・差出人を含まない。
        assertEquals("新着メールがあります", NotificationPlan.publicText());
        assertEquals("3 件の新着メール", NotificationPlan.summaryText(3));
        assertTrue(NotificationPlan.reconnectText("a@example.com").contains("a@example.com"));
        assertFalse(NotificationPlan.reconnectText("a@example.com").contains("token"));
    }

    @Test
    public void extraKeysAndActionAreNamespaced() {
        assertTrue(NotificationPlan.EXTRA_ACCOUNT_ID.startsWith("hitomado."));
        assertTrue(NotificationPlan.EXTRA_THREAD_ID.startsWith("hitomado."));
        assertNotEquals(NotificationPlan.EXTRA_ACCOUNT_ID, NotificationPlan.EXTRA_THREAD_ID);
        assertTrue(NotificationPlan.ACTION_OPEN_THREAD.startsWith("jp.ousias.hitomado."));
    }
}
