package com.wails.app;

import android.content.Context;
import android.util.Log;

import java.io.File;

/**
 * Go のヘッドレス同期（internal/headless。「1 回だけ同期して新着を返す」）を呼ぶ JNI の入口。
 *
 * <p>WorkManager の Worker は、アプリのプロセスが無ければ OS に起こされて動く。そのとき Activity も Wails のアプリも無いが、
 * {@code System.loadLibrary("wails")} で libwails.so を読み込むと Go のランタイムが始まり（main は動かない）、
 * Go の関数（{@code Java_com_wails_app_HeadlessSync_*}。internal/androidbg）を直接呼べる。資格情報は {@link SecretBridge} を
 * Go が JNI で呼び戻して読む（SecureStore。Activity 不要）。アプリのプロセスが既に動いているときは、Go の Gate が
 * アプリ本体のスケジューラに同期させる（docs/environment/android.md §15）。
 */
final class HeadlessSync {
    private static final String TAG = "HeadlessSync";

    /** アプリのデータフォルダ（Go の datadir_android.go と同じ files/Hitomado）。 */
    static final String DATA_DIR_NAME = "Hitomado";

    private static volatile boolean loaded;

    private HeadlessSync() {}

    private static native String nativeSyncOnce(String dataDir, String version, Object secrets);

    private static native void nativeCancel();

    static File dataDir(Context context) {
        return new File(context.getFilesDir(), DATA_DIR_NAME);
    }

    private static synchronized void ensureLoaded() {
        if (!loaded) {
            System.loadLibrary("wails");
            loaded = true;
        }
    }

    /** 全アカウントを 1 回同期して結果を返す。ライブラリを読めない・JNI が失敗したときは failed の結果。 */
    static SyncOutcome run(Context context) {
        try {
            ensureLoaded();
            String json = nativeSyncOnce(dataDir(context).getAbsolutePath(), BuildConfig.VERSION_NAME, new SecretBridge(context));
            return SyncOutcome.parse(json);
        } catch (Throwable t) { // UnsatisfiedLinkError を含む。例外のメッセージには出さず、クラス名だけ記録する
            Log.e(TAG, "headless sync failed: " + t.getClass().getName());
            return SyncOutcome.failed(SyncOutcome.ERROR_FAILED);
        }
    }

    /** 実行中の同期を取り消す（WorkManager が Worker を止めたとき）。ライブラリが未読み込みなら何もしない。 */
    static void cancel() {
        if (!loaded) {
            return;
        }
        try {
            nativeCancel();
        } catch (Throwable t) {
            Log.e(TAG, "cancel failed: " + t.getClass().getName());
        }
    }
}
