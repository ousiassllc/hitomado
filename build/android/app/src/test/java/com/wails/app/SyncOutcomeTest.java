package com.wails.app;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

/** Go のヘッドレス同期が返す JSON の読み取り（SyncOutcome）のテスト。 */
public class SyncOutcomeTest {

    @Test
    public void parsesNewMailAndAccounts() {
        SyncOutcome o = SyncOutcome.parse("{\"ok\":true,\"newMail\":[{\"accountId\":\"a\",\"accountEmail\":\"a@example.com\","
                + "\"threadId\":\"t1\",\"title\":\"山田\",\"body\":\"差出人\\n件名 \\ud83c\\udf89\"}],"
                + "\"accounts\":[{\"id\":\"a\",\"email\":\"a@example.com\",\"status\":\"ok\"},"
                + "{\"id\":\"b\",\"email\":\"b@example.com\",\"status\":\"needs_reauth\"}]}");
        assertTrue(o.ok);
        assertFalse(o.appRunning);
        assertEquals(1, o.newMail.size());
        SyncOutcome.NewMail m = o.newMail.get(0);
        assertEquals("a", m.accountId);
        assertEquals("a@example.com", m.accountEmail);
        assertEquals("t1", m.threadId);
        assertEquals("山田", m.title);
        assertEquals("差出人\n件名 \uD83C\uDF89", m.body);
        assertEquals(2, o.accounts.size());
        assertEquals(SyncOutcome.STATUS_NEEDS_REAUTH, o.accounts.get(1).status);
    }

    @Test
    public void appRunningFlagIsRead() {
        assertTrue(SyncOutcome.parse("{\"ok\":true,\"appRunning\":true,\"newMail\":[],\"accounts\":[]}").appRunning);
    }

    @Test
    public void failureCarriesOnlyTheCode() {
        SyncOutcome busy = SyncOutcome.parse("{\"ok\":false,\"error\":\"busy\",\"newMail\":[],\"accounts\":[]}");
        assertFalse(busy.ok);
        assertEquals(SyncOutcome.ERROR_BUSY, busy.error);
        assertEquals(SyncOutcome.ERROR_FAILED, SyncOutcome.parse("{\"ok\":false,\"error\":\"failed\"}").error);
        assertEquals("知らないコードは failed", SyncOutcome.ERROR_FAILED, SyncOutcome.parse("{\"ok\":false,\"error\":\"zzz\"}").error);
    }

    @Test
    public void brokenOrEmptyJsonIsFailed() {
        for (String s : new String[] {null, "", "not json", "[]", "{"}) {
            SyncOutcome o = SyncOutcome.parse(s);
            assertFalse("input=" + s, o.ok);
            assertEquals(SyncOutcome.ERROR_FAILED, o.error);
        }
    }

    @Test
    public void entriesWithoutIdsAreDropped() {
        SyncOutcome o = SyncOutcome.parse("{\"ok\":true,\"newMail\":[{\"accountId\":\"a\"},{\"threadId\":\"t\"}],"
                + "\"accounts\":[{\"email\":\"x\"}]}");
        assertTrue(o.ok);
        assertTrue(o.newMail.isEmpty());
        assertTrue(o.accounts.isEmpty());
    }

    @Test
    public void missingAccountStatusIsTreatedAsTransientError() {
        SyncOutcome o = SyncOutcome.parse("{\"ok\":true,\"accounts\":[{\"id\":\"a\"}]}");
        assertEquals(SyncOutcome.STATUS_ERROR, o.accounts.get(0).status);
    }
}
