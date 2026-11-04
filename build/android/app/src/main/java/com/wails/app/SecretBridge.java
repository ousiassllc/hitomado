package com.wails.app;

import android.content.Context;

import org.json.JSONObject;

/**
 * 資格情報の読み書きを、Go（internal/secrets の envelopeBridge）が使う「JSON の封筒」の形で提供する。
 * Activity に依存しない（{@link SecureStore} は applicationContext だけを使う）ため、WailsBridge（Activity のあるプロセス）と
 * WorkManager の Worker（{@link HeadlessSync}。Activity も Wails アプリも無い）が同じ実装・同じストアを使える。
 *
 * <p>戻り値は {"ok":true,"found":bool,"value":"…"}（get）／{"ok":true}（set・delete）／{"ok":false,"error":"<コード>"}。
 * コードは key_lost／failed／empty key だけで、値・キー・例外メッセージは含まない（#84）。
 */
public final class SecretBridge {
    private final SecureStore store;

    public SecretBridge(Context context) {
        this(SecureStore.getInstance(context));
    }

    SecretBridge(SecureStore store) {
        this.store = store;
    }

    private static String failure(String code) {
        return "{\"ok\":false,\"error\":\"" + code + "\"}";
    }

    /** json: {"key":"…","value":"…"}。 */
    public String secureSet(final String json) {
        try {
            JSONObject o = new JSONObject(json);
            String k = o.optString("key", "");
            if (k.isEmpty()) {
                return failure("empty key");
            }
            store.put(k, o.optString("value", ""));
            return "{\"ok\":true}";
        } catch (SecureStore.SecureStoreException e) {
            return failure(e.code());
        } catch (Exception e) {
            return failure("failed");
        }
    }

    public String secureGet(final String key) {
        try {
            if (key == null || key.isEmpty()) {
                return failure("empty key");
            }
            String value = store.get(key);
            if (value == null) {
                return "{\"ok\":true,\"found\":false}";
            }
            JSONObject result = new JSONObject(); // 値は JSON として符号化する（注入を防ぐ）
            result.put("ok", true);
            result.put("found", true);
            result.put("value", value);
            return result.toString();
        } catch (SecureStore.SecureStoreException e) {
            return failure(e.code());
        } catch (Exception e) {
            return failure("failed");
        }
    }

    public String secureDelete(final String key) {
        try {
            if (key == null || key.isEmpty()) {
                return failure("empty key");
            }
            store.remove(key);
            return "{\"ok\":true}";
        } catch (SecureStore.SecureStoreException e) {
            return failure(e.code());
        } catch (Exception e) {
            return failure("failed");
        }
    }
}
