package com.wails.app;

import java.io.File;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;

/**
 * 保存・開く操作で扱うパスとファイル名の検査（Android の API に依存しない。JVM で単体テストする）。
 * Go が渡すパスも、メールの添付のファイル名も「信頼できない入力」として扱い、許された場所の外・シンボリックリンク経由の
 * 脱出・区切り文字・制御文字を通さない。
 */
final class DocumentPaths {
    /** ファイル名の上限（バイト。UTF-8）。ext4／NTFS と同じ 255。 */
    static final int MAX_NAME_BYTES = 255;

    private DocumentPaths() {}

    /**
     * path が root の「直下のファイル」なら正規化したファイルを返し、そうでなければ null。
     * root の外・サブディレクトリ・存在しない・ディレクトリ・root の外を指すシンボリックリンクは null。
     */
    static File resolveChildFile(File root, String path) {
        File f = canonical(path);
        File r = canonical(root);
        if (f == null || r == null || !r.equals(f.getParentFile()) || !f.isFile()) {
            return null;
        }
        return f;
    }

    /** path が root の配下（深さは問わない）にあれば正規化したファイル（またはディレクトリ）を返し、そうでなければ null。 */
    static File resolveUnder(File root, String path) {
        File f = canonical(path);
        File r = canonical(root);
        if (f == null || r == null || f.equals(r)) {
            return null;
        }
        String prefix = r.getPath().endsWith(File.separator) ? r.getPath() : r.getPath() + File.separator;
        if (!f.getPath().startsWith(prefix) || !f.exists()) {
            return null;
        }
        return f;
    }

    private static File canonical(String path) {
        if (path == null || path.isEmpty() || path.indexOf('\0') >= 0) {
            return null;
        }
        return canonical(new File(path));
    }

    private static File canonical(File f) {
        try {
            return f.getCanonicalFile();
        } catch (IOException e) {
            return null;
        }
    }

    /**
     * 保存先の候補に渡すファイル名を安全にする。パス区切り・制御文字・コロンなどを _ にし、ディレクトリ部分は捨て、
     * 先頭と末尾の空白・ドットを除き、255 バイト以内（拡張子は保つ）にする。空になれば "file"。
     * Go 側（appcore.SanitizeFilename）でも直しているが、Java は Go を信用せず自分でも直す。
     */
    static String sanitizeName(String name) {
        if (name == null) {
            return "file";
        }
        int slash = Math.max(name.lastIndexOf('/'), name.lastIndexOf('\\'));
        String base = slash >= 0 ? name.substring(slash + 1) : name;
        StringBuilder sb = new StringBuilder(base.length());
        base.codePoints().forEach(cp -> {
            boolean bad = cp < 0x20 || cp == 0x7f || "<>:\"|?*".indexOf(cp) >= 0;
            sb.appendCodePoint(bad ? '_' : cp);
        });
        String s = trimDotsAndSpaces(sb.toString());
        if (s.isEmpty()) {
            return "file";
        }
        return truncateKeepingExtension(s);
    }

    private static String trimDotsAndSpaces(String s) {
        int start = 0;
        int end = s.length();
        while (start < end && (s.charAt(start) == ' ' || s.charAt(start) == '.')) start++;
        while (end > start && (s.charAt(end - 1) == ' ' || s.charAt(end - 1) == '.')) end--;
        return s.substring(start, end);
    }

    private static String truncateKeepingExtension(String s) {
        if (utf8Length(s) <= MAX_NAME_BYTES) {
            return s;
        }
        int dot = s.lastIndexOf('.');
        String ext = dot > 0 && utf8Length(s.substring(dot)) <= 32 ? s.substring(dot) : "";
        String base = s.substring(0, s.length() - ext.length());
        StringBuilder sb = new StringBuilder();
        int budget = MAX_NAME_BYTES - utf8Length(ext);
        int used = 0;
        for (int i = 0; i < base.length(); ) {
            int cp = base.codePointAt(i);
            int n = utf8Length(new String(Character.toChars(cp)));
            if (used + n > budget) break;
            sb.appendCodePoint(cp);
            used += n;
            i += Character.charCount(cp);
        }
        String head = trimDotsAndSpaces(sb.toString());
        return (head.isEmpty() ? "file" : head) + ext;
    }

    private static int utf8Length(String s) {
        return s.getBytes(java.nio.charset.StandardCharsets.UTF_8).length;
    }

    /**
     * JSON 文字列の ASCII 以外を \\uXXXX にする。JNI（Go 側の GetStringUTFChars）は Java の「modified UTF-8」を返し、
     * 標準の UTF-8 とは補助文字（絵文字など）の表現が違うので、Go へ返す JSON は ASCII だけにして、Go の JSON パーサーに戻させる。
     */
    static String escapeNonAscii(String json) {
        StringBuilder sb = new StringBuilder(json.length());
        for (int i = 0; i < json.length(); i++) {
            char c = json.charAt(i);
            if (c < 0x80) {
                sb.append(c);
            } else {
                sb.append(String.format("\\u%04x", (int) c));
            }
        }
        return sb.toString();
    }

    /** in を out へ最後までコピーする。どちらも閉じない。コピーしたバイト数を返す。 */
    static long copy(InputStream in, OutputStream out) throws IOException {
        byte[] buf = new byte[64 * 1024];
        long total = 0;
        int n;
        while ((n = in.read(buf)) >= 0) {
            out.write(buf, 0, n);
            total += n;
        }
        return total;
    }
}
