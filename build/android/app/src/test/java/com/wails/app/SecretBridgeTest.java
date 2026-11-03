package com.wails.app;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.json.JSONObject;
import org.junit.Test;

import java.io.IOException;
import java.security.InvalidKeyException;
import java.util.HashMap;
import java.util.Map;

/** SecretBridge（Go の envelopeBridge が読む JSON の封筒）のテスト。WailsBridge（Activity あり）と Worker（なし）が共有する。 */
public class SecretBridgeTest {
    private static final class MapBackend implements SecureStore.Backend {
        final Map<String, String> m = new HashMap<>();
        Exception fail;

        @Override
        public String get(String key) throws Exception {
            if (fail != null) throw fail;
            return m.get(key);
        }

        @Override
        public void put(String key, String value) throws Exception {
            if (fail != null) throw fail;
            m.put(key, value);
        }

        @Override
        public void remove(String key) throws Exception {
            if (fail != null) throw fail;
            m.remove(key);
        }

        @Override
        public void reset() {
            m.clear();
            fail = null;
        }
    }

    @Test
    public void roundTripWithoutAnyActivity() throws Exception {
        SecretBridge b = new SecretBridge(new SecureStore(new MapBackend()));
        assertEquals("{\"ok\":true,\"found\":false}", b.secureGet("k"));
        assertEquals("{\"ok\":true}", b.secureSet(new JSONObject().put("key", "k").put("value", "1//トークン\"x").toString()));
        JSONObject got = new JSONObject(b.secureGet("k"));
        assertTrue(got.getBoolean("ok"));
        assertTrue(got.getBoolean("found"));
        assertEquals("1//トークン\"x", got.getString("value"));
        assertEquals("{\"ok\":true}", b.secureDelete("k"));
        assertEquals("{\"ok\":true,\"found\":false}", b.secureGet("k"));
    }

    @Test
    public void emptyKeysAreRejected() {
        SecretBridge b = new SecretBridge(new SecureStore(new MapBackend()));
        assertEquals("{\"ok\":false,\"error\":\"empty key\"}", b.secureGet(""));
        assertEquals("{\"ok\":false,\"error\":\"empty key\"}", b.secureGet(null));
        assertEquals("{\"ok\":false,\"error\":\"empty key\"}", b.secureDelete(""));
        assertEquals("{\"ok\":false,\"error\":\"empty key\"}", b.secureSet("{\"key\":\"\",\"value\":\"v\"}"));
    }

    @Test
    public void failuresCarryOnlyCodes() {
        MapBackend backend = new MapBackend();
        SecretBridge b = new SecretBridge(new SecureStore(backend));
        backend.fail = new IOException("disk full: secret-value");
        String r = b.secureGet("k");
        assertEquals("{\"ok\":false,\"error\":\"failed\"}", r);
        assertFalse(r.contains("secret-value"));

        backend.fail = new InvalidKeyException("key gone");
        assertEquals("{\"ok\":false,\"error\":\"key_lost\"}", b.secureGet("k"));
    }

    @Test
    public void malformedSetJsonIsFailed() {
        SecretBridge b = new SecretBridge(new SecureStore(new MapBackend()));
        assertEquals("{\"ok\":false,\"error\":\"failed\"}", b.secureSet("not json"));
    }
}
