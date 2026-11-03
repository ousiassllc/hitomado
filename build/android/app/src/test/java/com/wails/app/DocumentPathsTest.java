package com.wails.app;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNotNull;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;

import org.junit.Rule;
import org.junit.Test;
import org.junit.rules.TemporaryFolder;

import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.Arrays;

/** DocumentPaths のユニットテスト（JVM）。パスの検査・ファイル名の安全化・コピー。 */
public class DocumentPathsTest {
    @Rule public TemporaryFolder tmp = new TemporaryFolder();

    private File write(File dir, String name) throws IOException {
        File f = new File(dir, name);
        Files.write(f.toPath(), "x".getBytes(StandardCharsets.UTF_8));
        return f;
    }

    @Test public void resolveChildFile_acceptsDirectChild() throws IOException {
        File root = tmp.newFolder("exports");
        File f = write(root, "save-1");
        assertEquals(f.getCanonicalFile(), DocumentPaths.resolveChildFile(root, f.getPath()));
    }

    @Test public void resolveChildFile_rejectsEscapes() throws IOException {
        File root = tmp.newFolder("exports");
        File outside = write(tmp.getRoot(), "secret");
        File sub = tmp.newFolder("exports", "sub");
        File nested = write(sub, "nested");
        write(root, "ok");

        assertNull("../ で外へ出る", DocumentPaths.resolveChildFile(root, root.getPath() + "/../secret"));
        assertNull("外のファイル", DocumentPaths.resolveChildFile(root, outside.getPath()));
        assertNull("サブディレクトリの中", DocumentPaths.resolveChildFile(root, nested.getPath()));
        assertNull("ディレクトリ", DocumentPaths.resolveChildFile(root, sub.getPath()));
        assertNull("存在しない", DocumentPaths.resolveChildFile(root, new File(root, "missing").getPath()));
        assertNull("root 自身", DocumentPaths.resolveChildFile(root, root.getPath()));
        assertNull("空", DocumentPaths.resolveChildFile(root, ""));
        assertNull("null", DocumentPaths.resolveChildFile(root, null));
        assertNull("NUL", DocumentPaths.resolveChildFile(root, root.getPath() + "/ok\0.txt"));
        assertNull("名前の前方一致（exports-evil）", DocumentPaths.resolveChildFile(root, new File(tmp.newFolder("exports-evil"), "f").getPath()));
    }

    @Test public void resolveChildFile_rejectsSymlinkOutsideRoot() throws IOException {
        File root = tmp.newFolder("exports");
        File outside = write(tmp.getRoot(), "secret");
        File link = new File(root, "link");
        try {
            Files.createSymbolicLink(link.toPath(), outside.toPath());
        } catch (UnsupportedOperationException | IOException e) {
            return; // シンボリックリンクを作れない環境（Windows の権限なし）では検査できない
        }
        assertNull(DocumentPaths.resolveChildFile(root, link.getPath()));
    }

    @Test public void resolveUnder_acceptsNestedAndRejectsOutside() throws IOException {
        File root = tmp.newFolder("logs");
        File f = write(root, "hitomado.log");
        File sub = tmp.newFolder("logs", "old");
        File nested = write(sub, "a.log");
        File outside = write(tmp.getRoot(), "secret");

        assertNotNull(DocumentPaths.resolveUnder(root, f.getPath()));
        assertNotNull(DocumentPaths.resolveUnder(root, nested.getPath()));
        assertNotNull("ディレクトリも解決する（開くかどうかは呼び出し側）", DocumentPaths.resolveUnder(root, sub.getPath()));
        assertNull("root 自身", DocumentPaths.resolveUnder(root, root.getPath()));
        assertNull(DocumentPaths.resolveUnder(root, outside.getPath()));
        assertNull(DocumentPaths.resolveUnder(root, root.getPath() + "/../secret"));
        assertNull(DocumentPaths.resolveUnder(root, new File(root, "missing").getPath()));
        assertNull("前方一致", DocumentPaths.resolveUnder(root, new File(tmp.newFolder("logs-evil"), "f").getPath()));
        assertNull(DocumentPaths.resolveUnder(root, null));
    }

    @Test public void sanitizeName_removesSeparatorsAndControlChars() {
        assertEquals("report.pdf", DocumentPaths.sanitizeName("report.pdf"));
        assertEquals("passwd", DocumentPaths.sanitizeName("../../etc/passwd"));
        assertEquals("win.ini", DocumentPaths.sanitizeName("..\\..\\Windows\\win.ini"));
        assertEquals("a_b_c.pdf", DocumentPaths.sanitizeName("a\u0000b\nc.pdf"));
        assertEquals("re_port__.pdf", DocumentPaths.sanitizeName("re:port*?.pdf"));
        assertEquals("請求書.pdf", DocumentPaths.sanitizeName("請求書.pdf"));
        assertEquals("file", DocumentPaths.sanitizeName(""));
        assertEquals("file", DocumentPaths.sanitizeName(".."));
        assertEquals("file", DocumentPaths.sanitizeName("a/b/.."));
        assertEquals("file", DocumentPaths.sanitizeName(null));
        assertEquals("a.txt", DocumentPaths.sanitizeName("  a.txt . "));
    }

    @Test public void sanitizeName_limitsBytesAndKeepsExtension() {
        char[] c = new char[400];
        Arrays.fill(c, 'a');
        String long1 = new String(c) + ".pdf";
        String out = DocumentPaths.sanitizeName(long1);
        assertTrue(out.getBytes(StandardCharsets.UTF_8).length <= 255);
        assertTrue(out.endsWith(".pdf"));

        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < 200; i++) sb.append("あ😀");
        String out2 = DocumentPaths.sanitizeName(sb + ".pdf");
        assertTrue(out2.getBytes(StandardCharsets.UTF_8).length <= 255);
        assertTrue(out2.endsWith(".pdf"));
        assertEquals("切り詰めて壊れたサロゲートを残さない", out2, new String(out2.getBytes(StandardCharsets.UTF_8), StandardCharsets.UTF_8));
    }

    @Test public void escapeNonAscii_roundTripsSurrogatePairs() {
        assertEquals("{\"name\":\"a\\u8acb\\ud83d\\ude00.pdf\"}", DocumentPaths.escapeNonAscii("{\"name\":\"a請😀.pdf\"}"));
        assertEquals("plain", DocumentPaths.escapeNonAscii("plain"));
    }

    @Test public void copy_streamsAllBytes() throws IOException {
        byte[] data = new byte[300_000];
        for (int i = 0; i < data.length; i++) data[i] = (byte) i;
        ByteArrayOutputStream out = new ByteArrayOutputStream();
        assertEquals(data.length, DocumentPaths.copy(new ByteArrayInputStream(data), out));
        assertTrue(Arrays.equals(data, out.toByteArray()));
        assertFalse(out.size() == 0);
    }
}
