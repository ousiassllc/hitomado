package com.wails.app;

import static org.junit.Assert.assertArrayEquals;
import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;

import org.junit.Before;
import org.junit.Rule;
import org.junit.Test;
import org.junit.rules.TemporaryFolder;

import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.IOException;
import java.io.OutputStream;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.ArrayList;
import java.util.List;

/**
 * DocumentExporter のユニットテスト（JVM）。Intent・ContentResolver は Host のフェイクにして、
 * 保存の状態（選ぶ・キャンセル・コピー・取りやめ・失敗）と、Uri へのコピー、パスの検査を確かめる。
 * SAF の画面そのものと Activity の結果の実機での受け渡しは、docs/environment/android.md §13 の手順で確かめる。
 */
public class DocumentExporterTest {
    @Rule public TemporaryFolder tmp = new TemporaryFolder();

    /** 同期実行の Host。runAsync もその場で実行する（待ちを使わず決定的にする）。 */
    static final class FakeHost implements DocumentExporter.Host {
        File exports;
        final List<String> events = new ArrayList<>();
        final List<Object> deleted = new ArrayList<>();
        final List<String[]> launched = new ArrayList<>();
        final ByteArrayOutputStream sink = new ByteArrayOutputStream();
        Exception launchError;
        boolean failOpen;
        boolean failWrite;

        @Override public File exportsDir() { return exports; }
        @Override public void runOnMain(Runnable r) { r.run(); }
        @Override public void runAsync(Runnable r) { r.run(); }
        @Override public void launchPicker(String name, String mime) throws Exception {
            launched.add(new String[]{name, mime});
            if (launchError != null) throw launchError;
        }
        @Override public OutputStream openOutput(Object uri) throws Exception {
            if (failOpen) throw new IOException("denied");
            return new OutputStream() {
                @Override public void write(int b) throws IOException {
                    if (failWrite) throw new IOException("disk full");
                    sink.write(b);
                }
                @Override public void write(byte[] b, int off, int len) throws IOException {
                    if (failWrite) throw new IOException("disk full");
                    sink.write(b, off, len);
                }
            };
        }
        @Override public String displayName(Object uri) { return "chosen-" + uri + ".pdf"; }
        @Override public void deleteDocument(Object uri) { deleted.add(uri); }
        @Override public void emit(int id, String status, String name, String error) {
            events.add(id + ":" + status + ":" + name + ":" + error);
        }
    }

    private FakeHost host;
    private DocumentExporter ex;

    @Before public void setUp() throws IOException {
        host = new FakeHost();
        host.exports = tmp.newFolder("exports");
        ex = new DocumentExporter(); // INSTANCE を共有しない
    }

    private String temp(String name, String content) throws IOException {
        File f = new File(host.exports, name);
        Files.write(f.toPath(), content.getBytes(StandardCharsets.UTF_8));
        return f.getPath();
    }

    @Test public void saveFlow_pickCopyEmitsSavedAndFreesSession() throws IOException {
        assertNull(ex.begin(host, 1, "../a:b.pdf", "application/pdf"));
        assertEquals("名前は Java 側でも安全にする", "a_b.pdf", host.launched.get(0)[0]);
        assertEquals("application/pdf", host.launched.get(0)[1]);

        ex.onPickerResult(host, "uri1");
        assertEquals("1:picked:null:null", host.events.get(0));

        assertNull(ex.commit(host, 1, temp("save-1", "DATA")));
        assertEquals("DATA", new String(host.sink.toByteArray(), StandardCharsets.UTF_8));
        assertEquals("1:saved:chosen-uri1.pdf:null", host.events.get(1));
        assertTrue(host.deleted.isEmpty());

        assertNull("コピー後は次の保存を始められる", ex.begin(host, 2, "b.txt", ""));
        assertEquals("MIME が空なら octet-stream", "application/octet-stream", host.launched.get(1)[1]);
    }

    @Test public void cancel_emitsCancelledAndFreesSession() {
        ex.begin(host, 1, "a.pdf", "application/pdf");
        ex.onPickerResult(host, null);
        assertEquals("1:cancelled:null:null", host.events.get(0));
        assertNull(ex.begin(host, 2, "a.pdf", "application/pdf"));
    }

    @Test public void busy_whileAnotherSaveIsInProgress() {
        assertNull(ex.begin(host, 1, "a.pdf", "application/pdf"));
        assertEquals(DocumentExporter.ERR_BUSY, ex.begin(host, 2, "b.pdf", "application/pdf"));
    }

    @Test public void launchFailure_emitsFailedAndFreesSession() {
        host.launchError = new DocumentExporter.NoPickerException();
        assertNull(ex.begin(host, 1, "a.pdf", "application/pdf"));
        assertEquals("1:failed:null:no_picker", host.events.get(0));
        host.launchError = new IllegalStateException("x");
        assertNull(ex.begin(host, 2, "a.pdf", "application/pdf"));
        assertEquals("2:failed:null:launch_failed", host.events.get(1));
        host.launchError = null;
        assertNull(ex.begin(host, 3, "a.pdf", "application/pdf"));
    }

    @Test public void commit_rejectsBadStateAndBadPath() throws IOException {
        String p = temp("save-1", "x");
        assertEquals("セッション無し", DocumentExporter.ERR_NO_SESSION, ex.commit(host, 1, p));
        ex.begin(host, 1, "a.pdf", "application/pdf");
        assertEquals("まだ選ばれていない", DocumentExporter.ERR_NO_SESSION, ex.commit(host, 1, p));
        ex.onPickerResult(host, "uri");
        assertEquals("id 違い", DocumentExporter.ERR_NO_SESSION, ex.commit(host, 2, p));
        File outside = tmp.newFile("secret");
        assertEquals("exports の外", DocumentExporter.ERR_BAD_PATH, ex.commit(host, 1, outside.getPath()));
        assertEquals("../ で外へ", DocumentExporter.ERR_BAD_PATH, ex.commit(host, 1, host.exports.getPath() + "/../secret"));
        assertEquals("存在しない", DocumentExporter.ERR_BAD_PATH, ex.commit(host, 1, new File(host.exports, "nope").getPath()));
        assertEquals("不正なパスでセッションは壊れない", 0, host.sink.size());
        assertNull(ex.commit(host, 1, p));
    }

    @Test public void copyFailure_deletesPartialDocumentAndEmitsFailed() throws IOException {
        ex.begin(host, 1, "a.pdf", "application/pdf");
        ex.onPickerResult(host, "uri1");
        host.failWrite = true;
        assertNull(ex.commit(host, 1, temp("save-1", "DATA")));
        assertEquals("1:failed:null:write_failed", host.events.get(1));
        assertEquals("途中までの保存先を残さない", 1, host.deleted.size());
        assertNull("失敗の後でも次の保存を始められる", ex.begin(host, 2, "a.pdf", "application/pdf"));
    }

    @Test public void openOutputFailure_isReportedWithoutLeakingDetails() throws IOException {
        ex.begin(host, 1, "a.pdf", "application/pdf");
        ex.onPickerResult(host, "uri1");
        host.failOpen = true;
        ex.commit(host, 1, temp("save-1", "DATA"));
        assertEquals("1:failed:null:write_failed", host.events.get(1));
    }

    @Test public void abort_afterPick_deletesEmptyDocument() throws IOException {
        ex.begin(host, 1, "a.pdf", "application/pdf");
        ex.onPickerResult(host, "uri1");
        ex.abort(host, 1);
        assertEquals(1, host.deleted.size());
        assertEquals("uri1", host.deleted.get(0));
        assertEquals("取りやめ後は commit できない", DocumentExporter.ERR_NO_SESSION, ex.commit(host, 1, temp("save-1", "x")));
        assertNull(ex.begin(host, 2, "a.pdf", "application/pdf"));
    }

    @Test public void abort_whilePickerOpen_discardsTheLateResult() {
        ex.begin(host, 1, "a.pdf", "application/pdf");
        ex.abort(host, 1);
        assertEquals("選択画面が出ている間は新しい保存を始められない", DocumentExporter.ERR_BUSY, ex.begin(host, 2, "b.pdf", "application/pdf"));
        ex.onPickerResult(host, "late-uri");
        assertEquals("遅れて選ばれた保存先は空のまま残さない", List.of("late-uri"), host.deleted);
        assertTrue("取りやめた保存の結果は Go へ知らせない", host.events.isEmpty());
        assertNull(ex.begin(host, 2, "b.pdf", "application/pdf"));
    }

    @Test public void abort_ofUnknownIdIsIgnored() {
        ex.begin(host, 1, "a.pdf", "application/pdf");
        ex.abort(host, 99);
        ex.onPickerResult(host, "uri1");
        assertEquals("1:picked:null:null", host.events.get(0));
    }

    @Test public void strayPickerResult_deletesChosenDocumentAndEmitsNothing() {
        ex.onPickerResult(host, "stray"); // プロセスが作り直された後などに届く、待っていない結果
        assertEquals(List.of("stray"), host.deleted);
        assertTrue(host.events.isEmpty());
        ex.onPickerResult(host, null);
        assertEquals(1, host.deleted.size());
    }

    @Test public void largeFile_isStreamedAndNotCorrupted() throws IOException {
        byte[] data = new byte[3 * 1024 * 1024 + 17];
        new java.util.Random(7).nextBytes(data);
        File f = new File(host.exports, "save-1");
        Files.write(f.toPath(), data);
        ex.begin(host, 1, "big.bin", "application/octet-stream");
        ex.onPickerResult(host, "uri1");
        assertNull(ex.commit(host, 1, f.getPath()));
        assertArrayEquals(data, host.sink.toByteArray());
        assertFalse(host.events.isEmpty());
    }
}
