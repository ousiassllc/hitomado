package com.wails.app;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;
import static org.junit.Assert.fail;

import org.junit.Test;

import java.io.IOException;
import java.security.InvalidKeyException;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;

import javax.crypto.AEADBadTagException;

/**
 * SecureStore のユニットテスト（JVM。端末・Context・Keystore は不要）。
 * 暗号化は EncryptedSharedPreferences の責務なので、ここでは保存先（Backend）をフェイクにして、
 * 値の往復・鍵の喪失からの復旧・Activity なしで使えること・スレッド安全性を確かめる。
 * 実機（Keystore を含む）の動作は docs/environment/android.md の手順で確かめる。
 */
public class SecureStoreTest {

    /** メモリ上のフェイク。failNext に入れた例外を次の 1 回だけ投げる（Keystore の鍵の喪失の模擬）。 */
    static final class FakeBackend implements SecureStore.Backend {
        final Map<String, String> data = new HashMap<>();
        final List<Exception> failNext = new ArrayList<>();
        int resets = 0;
        int concurrent = 0;
        int maxConcurrent = 0;

        private void maybeFail() throws Exception {
            if (!failNext.isEmpty()) {
                throw failNext.remove(0);
            }
        }

        private void enter() {
            concurrent++;
            maxConcurrent = Math.max(maxConcurrent, concurrent);
        }

        @Override
        public String get(String key) throws Exception {
            enter();
            try {
                maybeFail();
                return data.get(key);
            } finally {
                concurrent--;
            }
        }

        @Override
        public void put(String key, String value) throws Exception {
            enter();
            try {
                maybeFail();
                data.put(key, value);
            } finally {
                concurrent--;
            }
        }

        @Override
        public void remove(String key) throws Exception {
            enter();
            try {
                maybeFail();
                data.remove(key);
            } finally {
                concurrent--;
            }
        }

        @Override
        public void reset() {
            resets++;
            data.clear();
        }
    }

    private static String repeat(String s, int n) {
        StringBuilder b = new StringBuilder();
        for (int i = 0; i < n; i++) {
            b.append(s);
        }
        return b.toString();
    }

    @Test
    public void roundTripOverwriteAndDelete() throws Exception {
        SecureStore s = new SecureStore(new FakeBackend());
        assertNull(s.get("hitomado/refresh_token/a"));
        s.put("hitomado/refresh_token/a", "v1");
        assertEquals("v1", s.get("hitomado/refresh_token/a"));
        s.put("hitomado/refresh_token/a", "v2");
        assertEquals("v2", s.get("hitomado/refresh_token/a"));
        s.remove("hitomado/refresh_token/a");
        assertNull(s.get("hitomado/refresh_token/a"));
        s.remove("hitomado/refresh_token/a"); // 無くてもエラーにしない
    }

    @Test
    public void largeValueRoundTrips() throws Exception {
        SecureStore s = new SecureStore(new FakeBackend());
        String big = repeat("a1b2", 4096) + "終"; // 16KB 超
        s.put("hitomado/chatgpt_token/access", big);
        assertEquals(big, s.get("hitomado/chatgpt_token/access"));
    }

    @Test
    public void emptyKeyIsRejected() {
        SecureStore s = new SecureStore(new FakeBackend());
        try {
            s.get("");
            fail();
        } catch (SecureStore.SecureStoreException e) {
            assertEquals(SecureStore.Kind.FAILED, e.kind());
        }
    }

    @Test
    public void getAfterKeyLossResetsStoreAndReportsKeyLost() throws Exception {
        FakeBackend b = new FakeBackend();
        SecureStore s = new SecureStore(b);
        s.put("k1", "v1");
        s.put("k2", "v2");
        b.failNext.add(new AEADBadTagException("bad tag"));
        try {
            s.get("k1");
            fail("key loss must be reported");
        } catch (SecureStore.SecureStoreException e) {
            assertEquals(SecureStore.Kind.KEY_LOST, e.kind());
            assertEquals("key_lost", e.code());
            assertFalse(e.getMessage().contains("v1"));
        }
        assertEquals(1, b.resets);
        // 作り直し済み: 古い値は全部消え、新しい値を保存して読める。
        assertNull(s.get("k2"));
        s.put("k1", "v1-new");
        assertEquals("v1-new", s.get("k1"));
    }

    @Test
    public void wrappedKeyInvalidationIsKeyLost() throws Exception {
        // EncryptedSharedPreferences は復号の失敗を SecurityException に包む。
        FakeBackend b = new FakeBackend();
        SecureStore s = new SecureStore(b);
        b.failNext.add(new SecurityException("Could not decrypt value", new InvalidKeyException("Key permanently invalidated")));
        try {
            s.get("k");
            fail();
        } catch (SecureStore.SecureStoreException e) {
            assertEquals(SecureStore.Kind.KEY_LOST, e.kind());
        }
        assertEquals(1, b.resets);
    }

    @Test
    public void putAfterKeyLossResetsAndRetriesOnce() throws Exception {
        FakeBackend b = new FakeBackend();
        SecureStore s = new SecureStore(b);
        s.put("old", "x");
        b.failNext.add(new InvalidKeyException("Failed to unwrap key"));
        s.put("new", "y"); // 例外にならず、作り直したストアへ保存される
        assertEquals(1, b.resets);
        assertNull(s.get("old"));
        assertEquals("y", s.get("new"));
    }

    @Test
    public void removeAfterKeyLossResetsAndSucceeds() throws Exception {
        FakeBackend b = new FakeBackend();
        SecureStore s = new SecureStore(b);
        s.put("k", "v");
        b.failNext.add(new AEADBadTagException());
        s.remove("k");
        assertEquals(1, b.resets);
        assertNull(s.get("k"));
    }

    @Test
    public void ioFailureDoesNotWipeStore() throws Exception {
        FakeBackend b = new FakeBackend();
        SecureStore s = new SecureStore(b);
        s.put("k", "v");
        b.failNext.add(new IOException("commit failed"));
        try {
            s.get("k");
            fail();
        } catch (SecureStore.SecureStoreException e) {
            assertEquals(SecureStore.Kind.FAILED, e.kind());
        }
        assertEquals(0, b.resets);
        assertEquals("v", s.get("k"));
    }

    @Test
    public void keyLostClassification() {
        assertTrue(SecureStore.isKeyLost(new InvalidKeyException()));
        assertTrue(SecureStore.isKeyLost(new AEADBadTagException()));
        assertTrue(SecureStore.isKeyLost(new RuntimeException(new java.security.UnrecoverableKeyException())));
        assertTrue(SecureStore.isKeyLost(new java.security.ProviderException("Keystore operation failed")));
        assertFalse(SecureStore.isKeyLost(new IOException("disk")));
        assertFalse(SecureStore.isKeyLost(new IllegalStateException()));
        assertFalse(SecureStore.isKeyLost(new SecureStore.SecureStoreException(SecureStore.Kind.UNAVAILABLE)));
    }

    /** Activity も Context も無いスレッド（Worker 相当）から、同じ SecureStore を並行に使っても壊れない。 */
    @Test
    public void concurrentUseFromBackgroundThreadsIsSerialized() throws Exception {
        FakeBackend b = new FakeBackend();
        final SecureStore s = new SecureStore(b);
        int threads = 8;
        ExecutorService pool = Executors.newFixedThreadPool(threads);
        final CountDownLatch start = new CountDownLatch(1);
        List<Future<?>> fs = new ArrayList<>();
        for (int t = 0; t < threads; t++) {
            final int id = t;
            fs.add(pool.submit(() -> {
                start.await();
                for (int i = 0; i < 200; i++) {
                    String k = "k" + id;
                    s.put(k, "v" + i);
                    assertEquals("v" + i, s.get(k));
                }
                return null;
            }));
        }
        start.countDown();
        for (Future<?> f : fs) {
            f.get();
        }
        pool.shutdown();
        assertEquals(1, b.maxConcurrent);
    }
}
