package com.wails.app;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.HashSet;
import java.util.List;
import java.util.Set;

/** Worker の判断（通知・戻り値・再接続通知の 1 回だけ。SyncPlan）のテスト。 */
public class SyncPlanTest {
    private static SyncOutcome.Account acct(String id, String status) {
        return new SyncOutcome.Account(id, id + "@example.com", status);
    }

    private static SyncOutcome ok(List<SyncOutcome.NewMail> mail, SyncOutcome.Account... accounts) {
        return new SyncOutcome(true, "", false, mail, Arrays.asList(accounts));
    }

    private static List<SyncOutcome.NewMail> oneMail() {
        List<SyncOutcome.NewMail> l = new ArrayList<>();
        l.add(new SyncOutcome.NewMail("a", "a@example.com", "t1", "A", "from\nsubject"));
        return l;
    }

    private static Set<String> set(String... ids) {
        return new HashSet<>(Arrays.asList(ids));
    }

    @Test
    public void successNotifiesNewMail() {
        SyncPlan p = SyncPlan.of(ok(oneMail(), acct("a", "ok")), 0, set());
        assertEquals(SyncPlan.Result.SUCCESS, p.result);
        assertEquals(1, p.notify.size());
        assertTrue(p.reconnect.isEmpty());
    }

    @Test
    public void reconnectIsNotifiedOnlyOnce() {
        SyncOutcome o = ok(new ArrayList<SyncOutcome.NewMail>(), acct("a", "needs_reauth"));
        SyncPlan first = SyncPlan.of(o, 0, set());
        assertEquals(1, first.reconnect.size());
        assertEquals(SyncPlan.Result.SUCCESS, first.result); // 再試行しても直らない
        assertTrue(first.reconnectNotified.contains("a"));

        SyncPlan second = SyncPlan.of(o, 0, first.reconnectNotified);
        assertTrue("2 回目以降は通知しない", second.reconnect.isEmpty());
        assertTrue(second.reconnectNotified.contains("a"));
    }

    @Test
    public void reconnectRecordIsClearedWhenAccountSyncsAgain() {
        SyncPlan p = SyncPlan.of(ok(new ArrayList<SyncOutcome.NewMail>(), acct("a", "ok")), 0, set("a"));
        assertTrue(p.reconnectNotified.isEmpty());
        // その後また再接続が必要になれば、もう一度通知する。
        SyncPlan again = SyncPlan.of(ok(new ArrayList<SyncOutcome.NewMail>(), acct("a", "needs_reauth")), 0, p.reconnectNotified);
        assertEquals(1, again.reconnect.size());
    }

    @Test
    public void reconnectRecordOfRemovedAccountIsDropped() {
        SyncPlan p = SyncPlan.of(ok(new ArrayList<SyncOutcome.NewMail>(), acct("a", "ok")), 0, set("a", "gone"));
        assertEquals(Collections.<String>emptySet(), p.reconnectNotified);
    }

    @Test
    public void appRunningResultKeepsRecords() {
        SyncOutcome o = new SyncOutcome(true, "", true, new ArrayList<SyncOutcome.NewMail>(), new ArrayList<SyncOutcome.Account>());
        assertEquals(set("a"), SyncPlan.of(o, 0, set("a")).reconnectNotified);
    }

    @Test
    public void transientErrorsOnlyRetryUntilAttemptsRunOut() {
        SyncOutcome o = ok(new ArrayList<SyncOutcome.NewMail>(), acct("a", "error"), acct("b", "error"));
        assertEquals(SyncPlan.Result.RETRY, SyncPlan.of(o, 0, set()).result);
        assertEquals(SyncPlan.Result.RETRY, SyncPlan.of(o, 1, set()).result);
        assertEquals("次の定期実行に任せる", SyncPlan.Result.SUCCESS, SyncPlan.of(o, SyncPlan.MAX_ATTEMPTS - 1, set()).result);
    }

    @Test
    public void partialFailureIsSuccessAndStillNotifies() {
        SyncPlan p = SyncPlan.of(ok(oneMail(), acct("a", "ok"), acct("b", "error")), 0, set());
        assertEquals(SyncPlan.Result.SUCCESS, p.result);
        assertEquals(1, p.notify.size());
    }

    @Test
    public void failedRunRetriesThenGivesUp() {
        SyncOutcome failed = SyncOutcome.failed(SyncOutcome.ERROR_FAILED);
        assertEquals(SyncPlan.Result.RETRY, SyncPlan.of(failed, 0, set("a")).result);
        assertEquals(SyncPlan.Result.SUCCESS, SyncPlan.of(failed, SyncPlan.MAX_ATTEMPTS - 1, set()).result);
        assertEquals("失敗では記録を変えない", set("a"), SyncPlan.of(failed, 0, set("a")).reconnectNotified);
        assertTrue(SyncPlan.of(failed, 0, set()).notify.isEmpty());
    }

    @Test
    public void busyIsSuccess() {
        assertEquals(SyncPlan.Result.SUCCESS, SyncPlan.of(SyncOutcome.failed(SyncOutcome.ERROR_BUSY), 0, set()).result);
    }

    @Test
    public void noAccountsIsSuccessWithNothingToDo() {
        SyncPlan p = SyncPlan.of(ok(new ArrayList<SyncOutcome.NewMail>()), 0, set());
        assertEquals(SyncPlan.Result.SUCCESS, p.result);
        assertTrue(p.notify.isEmpty());
        assertTrue(p.reconnect.isEmpty());
    }
}
