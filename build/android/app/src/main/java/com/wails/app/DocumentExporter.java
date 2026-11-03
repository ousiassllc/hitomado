package com.wails.app;

import java.io.File;
import java.io.FileInputStream;
import java.io.InputStream;
import java.io.OutputStream;

/**
 * 「保存先を利用者に選ばせて、Go が書いたファイルをそこへコピーする」流れの状態（Android の API に依存しない。
 * 実際の Intent・ContentResolver は {@link Host} の実装（WailsBridge）が担い、ここは JVM で単体テストする。Issue #85）。
 *
 * <pre>
 *   Go: begin(id, name, mime) ─▶ picker ─▶ {@link #onPickerResult}  ── イベント picked / cancelled ─▶ Go
 *   Go: 一時ファイル（cache/exports の直下）へ書く
 *   Go: commit(id, path) ─▶ 別スレッドで一時ファイル → 選ばれた Uri へストリームコピー ── イベント saved / failed ─▶ Go
 *   Go: abort(id)  （書き込み失敗・待ちの打ち切り。選ばれて空のままの保存先を消す）
 * </pre>
 *
 * 同時に 1 件だけ。状態はプロセス内の 1 つ（{@link #INSTANCE}）で、Activity が作り直されても（結果が新しい Activity に届いても）続く。
 * プロセスごと終わった場合は結果は捨てられる（Go 側の待ちも一緒に消える）。巨大な byte[] は JNI を通さず、
 * Go が書いたファイルを Java がパスから読む。
 */
final class DocumentExporter {
    static final DocumentExporter INSTANCE = new DocumentExporter();

    // イベントの status（internal/exportfile/bridge.go の Status と一致させる）。
    static final String STATUS_PICKED = "picked";
    static final String STATUS_CANCELLED = "cancelled";
    static final String STATUS_SAVED = "saved";
    static final String STATUS_FAILED = "failed";

    // 同期の呼び出しが返す失敗コード、および failed イベントの error（パス・Uri・例外メッセージは含めない）。
    static final String ERR_BUSY = "busy";
    static final String ERR_NO_SESSION = "no_session";
    static final String ERR_BAD_PATH = "bad_path";
    static final String ERR_NO_PICKER = "no_picker";
    static final String ERR_LAUNCH_FAILED = "launch_failed";
    static final String ERR_WRITE_FAILED = "write_failed";

    /** 保存先を選ぶ画面を出せるアプリ（ドキュメントプロバイダ）が無い。 */
    static final class NoPickerException extends Exception {
        NoPickerException() {
            super("no document picker");
        }
    }

    /** Android に依存する部分。 */
    interface Host {
        /** 一時ファイルを置く、アプリの cache 配下の専用ディレクトリ（Go が書き、Java が読む唯一の場所）。 */
        File exportsDir();

        /** メインスレッドで r を実行する（すでにメインスレッドならその場で）。 */
        void runOnMain(Runnable r);

        /** バックグラウンドで r を実行する。 */
        void runAsync(Runnable r);

        /**
         * ACTION_CREATE_DOCUMENT の画面を出す（メインスレッドで呼ばれる）。結果は {@link #onPickerResult} へ。
         * 出せるアプリが無ければ {@link NoPickerException}。
         */
        void launchPicker(String name, String mime) throws Exception;

        /** uri へ上書きで書く出力。 */
        OutputStream openOutput(Object uri) throws Exception;

        /** 保存先の表示名（取れなければ null）。 */
        String displayName(Object uri);

        /** 選ばれて空のままの保存先を消す（失敗は無視してよい）。 */
        void deleteDocument(Object uri);

        /** 結果を Go へ知らせる。name は saved のとき保存先の表示名、error は failed のときのコード（どちらも無ければ null）。 */
        void emit(int id, String status, String name, String error);
    }

    private enum State { PICKING, PICKED, COPYING }

    private static final class Session {
        final int id;
        State state = State.PICKING;
        Object uri;
        boolean abandoned; // Go が取りやめた。選択画面の結果が来たら保存先を消して捨てる

        Session(int id) {
            this.id = id;
        }
    }

    private Session session; // synchronized (this)

    /** 保存先の選択を始める。別の保存が進行中なら ERR_BUSY。成功は null。選択画面を出せなければ failed イベントで知らせる。 */
    String begin(Host host, int id, String name, String mime) {
        synchronized (this) {
            if (session != null) {
                return ERR_BUSY;
            }
            session = new Session(id);
        }
        final String safeName = DocumentPaths.sanitizeName(name);
        final String safeMime = mime == null || mime.isEmpty() ? "application/octet-stream" : mime;
        host.runOnMain(() -> {
            try {
                host.launchPicker(safeName, safeMime);
            } catch (NoPickerException e) {
                failPicking(host, id, ERR_NO_PICKER);
            } catch (Exception e) {
                failPicking(host, id, ERR_LAUNCH_FAILED);
            }
        });
        return null;
    }

    private void failPicking(Host host, int id, String code) {
        synchronized (this) {
            if (session == null || session.id != id) return;
            boolean abandoned = session.abandoned;
            session = null;
            if (abandoned) return;
        }
        host.emit(id, STATUS_FAILED, null, code);
    }

    /** 選択画面の結果。uri が null なら利用者がキャンセルした。 */
    void onPickerResult(Host host, Object uri) {
        final int id;
        final boolean abandoned;
        synchronized (this) {
            if (session == null || session.state != State.PICKING) {
                // 取りこぼした結果（プロセスが作り直された後など）。選ばれた保存先は空のまま残さない。
                if (uri != null) host.deleteDocument(uri);
                return;
            }
            id = session.id;
            abandoned = session.abandoned;
            if (uri == null || abandoned) {
                session = null;
            } else {
                session.uri = uri;
                session.state = State.PICKED;
            }
        }
        if (uri != null && abandoned) {
            host.deleteDocument(uri);
        }
        if (abandoned) {
            return;
        }
        host.emit(id, uri == null ? STATUS_CANCELLED : STATUS_PICKED, null, null);
    }

    /**
     * path（exportsDir の直下のファイル）を、id で選ばれた保存先へコピーさせる。コピーは別スレッドで、結果は saved／failed
     * イベント。同期で返す失敗: ERR_NO_SESSION（id が違う・まだ選ばれていない）、ERR_BAD_PATH（許された場所の外）。成功は null。
     */
    String commit(Host host, int id, String path) {
        File src = DocumentPaths.resolveChildFile(host.exportsDir(), path);
        final Object uri;
        synchronized (this) {
            if (session == null || session.id != id || session.state != State.PICKED) {
                return ERR_NO_SESSION;
            }
            if (src == null) {
                return ERR_BAD_PATH;
            }
            session.state = State.COPYING;
            uri = session.uri;
        }
        final File file = src;
        host.runAsync(() -> copyAndReport(host, id, file, uri));
        return null;
    }

    private void copyAndReport(Host host, int id, File src, Object uri) {
        boolean ok = false;
        try (InputStream in = new FileInputStream(src); OutputStream out = host.openOutput(uri)) {
            DocumentPaths.copy(in, out);
            out.flush();
            ok = true;
        } catch (Exception e) {
            // 失敗の詳細（パス・Uri を含みうる）は返さない。
        }
        if (!ok) {
            host.deleteDocument(uri); // 途中までのファイルを保存先に残さない
        }
        synchronized (this) {
            session = null;
        }
        if (ok) {
            host.emit(id, STATUS_SAVED, host.displayName(uri), null);
        } else {
            host.emit(id, STATUS_FAILED, null, ERR_WRITE_FAILED);
        }
    }

    /**
     * 保存を取りやめる。選ばれて空の保存先は消す。選択画面がまだ出ているときは閉じられないので、結果が来たら捨てる。
     * コピー中のものは止めない（終われば状態が空く）。進行中の保存が無ければ何もしない。
     */
    void abort(Host host, int id) {
        Object toDelete = null;
        synchronized (this) {
            if (session == null || session.id != id) return;
            if (session.state == State.PICKING) {
                session.abandoned = true;
            } else if (session.state == State.PICKED) {
                toDelete = session.uri;
                session = null;
            }
        }
        if (toDelete != null) host.deleteDocument(toDelete);
    }
}
