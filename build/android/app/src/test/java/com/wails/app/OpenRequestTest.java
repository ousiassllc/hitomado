package com.wails.app;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNotNull;
import static org.junit.Assert.assertNull;

import org.json.JSONObject;
import org.junit.After;
import org.junit.Test;

/** 通知のタップで開くスレッドの受け渡し（OpenRequest）のテスト。 */
public class OpenRequestTest {

    @After
    public void drain() {
        OpenRequest.take();
    }

    @Test
    public void ofRejectsMissingIds() {
        assertNull(OpenRequest.of(null, "t"));
        assertNull(OpenRequest.of("a", null));
        assertNull(OpenRequest.of("", "t"));
        assertNull(OpenRequest.of("a", ""));
        assertNotNull(OpenRequest.of("a", "t"));
    }

    @Test
    public void fromIntentIgnoresNull() {
        assertNull(OpenRequest.fromIntent(null));
    }

    @Test
    public void takeReturnsLatestOnceThenEmpty() {
        assertNull(OpenRequest.take());
        OpenRequest.set(OpenRequest.of("a", "t1"));
        OpenRequest.set(OpenRequest.of("a", "t2"));
        OpenRequest.set(null); // null は保持中の要求を消さない
        OpenRequest r = OpenRequest.take();
        assertNotNull(r);
        assertEquals("t2", r.threadId);
        assertNull("取り出すと空になる（二重に開かない）", OpenRequest.take());
    }

    @Test
    public void toJsonRoundTripsAndEscapes() throws Exception {
        OpenRequest r = OpenRequest.of("01HZX", "18c\"\\x\u00e9");
        JSONObject o = new JSONObject(r.toJson());
        assertEquals("01HZX", o.getString("accountId"));
        assertEquals("18c\"\\x\u00e9", o.getString("threadId"));
        for (char c : r.toJson().toCharArray()) {
            if (c > 0x7e) {
                throw new AssertionError("JNI の NewStringUTF のため ASCII だけにする");
            }
        }
    }
}
