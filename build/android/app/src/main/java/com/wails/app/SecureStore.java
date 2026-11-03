package com.wails.app;

import android.content.Context;
import android.content.SharedPreferences;
import android.os.Build;
import android.util.Log;

import androidx.security.crypto.EncryptedSharedPreferences;
import androidx.security.crypto.MasterKey;

import java.io.IOException;
import java.security.InvalidKeyException;
import java.security.KeyStore;
import java.security.KeyStoreException;
import java.security.ProviderException;
import java.security.UnrecoverableKeyException;

import javax.crypto.BadPaddingException;

/**
 * 資格情報（OAuth トークン・API キー）の保管先。EncryptedSharedPreferences（鍵は Android Keystore）。
 *
 * <p>Activity に依存しない（applicationContext だけを使う）ため、WorkManager の Worker など Activity の無い
 * バックグラウンドからも、Go（JNI 経由。WailsBridge）と同じストアを同じ鍵で読み書きできる。
 * 全メソッドは 1 つの monitor で直列化する（プロセス内で 1 インスタンス。{@link #getInstance}）。
 *
 * <p>Keystore の鍵が失われた（端末の移行・ロック解除方法のリセット・鍵の無効化などで復号できない）ときは、
 * 暗号化ストアを消して作り直し、{@link Kind#KEY_LOST} を投げる（起動は落とさない。入っていた値はすべて失われる）。
 * Go 側はこれを「資格情報が無い」と同じに扱い、再ログインへ案内する。
 *
 * <p>値・キーの中身はログにもエラーメッセージにも出さない（例外のクラス名だけを出す）。
 */
public final class SecureStore {
    private static final String TAG = "SecureStore";

    /** 暗号化ストア（SharedPreferences のファイル名）。Wails 既定の "wails_secure" とは分ける。 */
    static final String PREFS_NAME = "hitomado_secrets";
    /** Keystore 上のマスター鍵の別名。androidx 既定（_androidx_security_master_key_）を共有しない。 */
    static final String MASTER_KEY_ALIAS = "hitomado_secrets_master_key";

    private static final String ANDROID_KEYSTORE_EXCEPTION = "android.security.KeyStoreException";
    private static final int MAX_CAUSE_DEPTH = 16;

    /** 失敗の種類。{@link SecureStoreException#code()} の値は Go 側（internal/secrets）と一致させる。 */
    public enum Kind {
        /** Keystore の鍵が失われ、復号できない。ストアは作り直し済み。 */
        KEY_LOST("key_lost"),
        /** 暗号化ストアを使えない（API 23 未満）。 */
        UNAVAILABLE("unavailable"),
        /** それ以外（ディスク書き込みの失敗など）。ストアは消さない。 */
        FAILED("failed");

        final String code;

        Kind(String code) {
            this.code = code;
        }
    }

    public static final class SecureStoreException extends Exception {
        private final Kind kind;

        SecureStoreException(Kind kind) {
            // メッセージは種類だけ。元の例外は載せない（メッセージに値が含まれうるため。ログには例外のクラス名だけを出す）。
            super(kind.code);
            this.kind = kind;
        }

        public Kind kind() {
            return kind;
        }

        /** Go 側へ返すコード（"key_lost"／"unavailable"／"failed"）。 */
        public String code() {
            return kind.code;
        }
    }

    /** 保存先の抽象。本番は {@link AndroidBackend}、単体テストはフェイク。 */
    interface Backend {
        /** 無ければ null。 */
        String get(String key) throws Exception;

        void put(String key, String value) throws Exception;

        void remove(String key) throws Exception;

        /** 暗号化ストア（ファイル・メモリ上の保持）と Keystore の鍵を消す。 */
        void reset() throws Exception;
    }

    private static SecureStore instance;

    private final Backend backend;

    SecureStore(Backend backend) {
        this.backend = backend;
    }

    /** プロセス内で共有する 1 インスタンス。applicationContext だけを保持する。 */
    public static synchronized SecureStore getInstance(Context context) {
        if (instance == null) {
            instance = new SecureStore(new AndroidBackend(context.getApplicationContext()));
        }
        return instance;
    }

    /** 値を返す。無ければ null。 */
    public synchronized String get(String key) throws SecureStoreException {
        requireKey(key);
        try {
            return backend.get(key);
        } catch (Exception e) {
            throw failure(e);
        }
    }

    /** 値を保存する。鍵が失われていれば、ストアを作り直してから 1 回だけ書き直す。 */
    public synchronized void put(String key, String value) throws SecureStoreException {
        requireKey(key);
        try {
            backend.put(key, value);
            return;
        } catch (Exception e) {
            if (!isKeyLost(e)) {
                throw failure(e);
            }
            resetAfterKeyLoss(e);
        }
        try {
            backend.put(key, value);
        } catch (Exception e) {
            throw failure(e);
        }
    }

    /** 値を削除する。無くてもエラーにしない。鍵が失われていれば、ストアを作り直す（消えたのと同じ）。 */
    public synchronized void remove(String key) throws SecureStoreException {
        requireKey(key);
        try {
            backend.remove(key);
        } catch (Exception e) {
            if (!isKeyLost(e)) {
                throw failure(e);
            }
            resetAfterKeyLoss(e);
        }
    }

    private static void requireKey(String key) throws SecureStoreException {
        if (key == null || key.isEmpty()) {
            throw new SecureStoreException(Kind.FAILED);
        }
    }

    /** 失敗を種類に分ける。鍵の喪失ならストアを作り直したうえで KEY_LOST を返す。 */
    private SecureStoreException failure(Exception e) {
        if (e instanceof SecureStoreException) {
            return (SecureStoreException) e;
        }
        if (!isKeyLost(e)) {
            Log.e(TAG, "secure storage failed: " + e.getClass().getName());
            return new SecureStoreException(Kind.FAILED);
        }
        try {
            resetAfterKeyLoss(e);
        } catch (SecureStoreException resetFailure) {
            return resetFailure; // 作り直せなかった。次の呼び出しで再び作り直しを試みる。
        }
        return new SecureStoreException(Kind.KEY_LOST);
    }

    private void resetAfterKeyLoss(Exception cause) throws SecureStoreException {
        Log.w(TAG, "Keystore key lost; resetting secure storage (" + cause.getClass().getName() + ")");
        try {
            backend.reset();
        } catch (Exception resetFailure) {
            Log.e(TAG, "secure storage reset failed: " + resetFailure.getClass().getName());
            throw new SecureStoreException(Kind.FAILED);
        }
    }

    /**
     * Keystore の鍵が失われた・使えなくなったことによる失敗か（失敗の cause の連鎖をたどる。
     * EncryptedSharedPreferences は復号の失敗を SecurityException などに包むため）。
     *
     * <p>鍵の喪失とみなす: InvalidKeyException（KeyPermanentlyInvalidatedException を含む）、
     * BadPaddingException（AEADBadTagException を含む。鍵が別物で復号できない）、UnrecoverableKeyException、
     * java.security.KeyStoreException、ProviderException、android.security.KeyStoreException、
     * Tink のキーセットが壊れて読めない InvalidProtocolBufferException。
     * 鍵の喪失とみなさない: ディスクの IOException だけの失敗、それ以外の例外、
     * Keystore が一時的に使えないだけの失敗（isTransientFailure）。資格情報を消すのは確実なときだけにする。
     */
    static boolean isKeyLost(Throwable t) {
        if (hasTransientKeystoreFailure(t)) {
            return false;
        }
        int depth = 0;
        for (Throwable c = t; c != null && depth < MAX_CAUSE_DEPTH; c = c.getCause(), depth++) {
            if (c instanceof InvalidKeyException
                    || c instanceof BadPaddingException
                    || c instanceof UnrecoverableKeyException
                    || c instanceof KeyStoreException
                    || c instanceof ProviderException) {
                return true;
            }
            String name = c.getClass().getName();
            // android.security.KeyStoreException は API 33 以降の公開クラスだが、古い API では内部クラスなので名前で見る。
            if (ANDROID_KEYSTORE_EXCEPTION.equals(name) || name.endsWith(".InvalidProtocolBufferException")) {
                return true;
            }
        }
        return false;
    }

    /** 連鎖の中に、再試行すれば直りうる（一時的な）android.security.KeyStoreException があるか。 */
    private static boolean hasTransientKeystoreFailure(Throwable t) {
        int depth = 0;
        for (Throwable c = t; c != null && depth < MAX_CAUSE_DEPTH; c = c.getCause(), depth++) {
            if (!ANDROID_KEYSTORE_EXCEPTION.equals(c.getClass().getName())) {
                continue;
            }
            try {
                if (Boolean.TRUE.equals(c.getClass().getMethod("isTransientFailure").invoke(c))) {
                    return true;
                }
            } catch (ReflectiveOperationException | RuntimeException ignored) {
                // API 33 未満には無いメソッド。一時的かどうか分からないときは、鍵の喪失として扱う。
            }
        }
        return false;
    }

    /** EncryptedSharedPreferences と Android Keystore を使う本番の保存先（security-crypto は 1.1.0 で全 API が deprecated）。 */
    @SuppressWarnings("deprecation")
    static final class AndroidBackend implements Backend {
        private final Context context;
        private SharedPreferences prefs; // SecureStore の monitor で保護される

        AndroidBackend(Context context) {
            this.context = context;
        }

        private SharedPreferences prefs() throws Exception {
            if (prefs != null) {
                return prefs;
            }
            if (Build.VERSION.SDK_INT < Build.VERSION_CODES.M) {
                throw new SecureStoreException(Kind.UNAVAILABLE);
            }
            MasterKey key = new MasterKey.Builder(context, MASTER_KEY_ALIAS)
                    .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
                    .build();
            prefs = EncryptedSharedPreferences.create(context, PREFS_NAME, key,
                    EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
                    EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM);
            return prefs;
        }

        @Override
        public String get(String key) throws Exception {
            return prefs().getString(key, null);
        }

        @Override
        public void put(String key, String value) throws Exception {
            if (!prefs().edit().putString(key, value).commit()) {
                throw new IOException("commit failed");
            }
        }

        @Override
        public void remove(String key) throws Exception {
            if (!prefs().edit().remove(key).commit()) {
                throw new IOException("commit failed");
            }
        }

        @Override
        public void reset() throws Exception {
            prefs = null;
            // 暗号化せずに同じファイルを開いて空にする（Tink のキーセットもこのファイルにあるため一緒に消える）。
            // 先に空にするのは、deleteSharedPreferences が使えない API 23 でもメモリ上の保持を捨てるため。
            context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE).edit().clear().commit();
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.N) {
                context.deleteSharedPreferences(PREFS_NAME);
            }
            KeyStore ks = KeyStore.getInstance("AndroidKeyStore");
            ks.load(null);
            if (ks.containsAlias(MASTER_KEY_ALIAS)) {
                ks.deleteEntry(MASTER_KEY_ALIAS);
            }
        }
    }
}
