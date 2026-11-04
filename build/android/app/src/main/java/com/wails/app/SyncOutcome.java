package com.wails.app;

import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;

import java.util.ArrayList;
import java.util.Collections;
import java.util.List;

/**
 * Go のヘッドレス同期（internal/headless.Encode）が返す JSON の読み取り結果。Android に依存しない純 Java。
 *
 * <pre>
 * {"ok":true,"appRunning":false,
 *  "newMail":[{"accountId","accountEmail","threadId","title","body"}],
 *  "accounts":[{"id","email","status":"ok|needs_reauth|error"}]}
 * {"ok":false,"error":"busy|failed"}
 * </pre>
 *
 * 失敗の JSON は理由のコードだけで、件名・宛先などは含まれない。読めない JSON は {@code failed} の失敗として扱う。
 */
final class SyncOutcome {
    static final String STATUS_OK = "ok";
    static final String STATUS_NEEDS_REAUTH = "needs_reauth";
    static final String STATUS_ERROR = "error";

    static final String ERROR_BUSY = "busy";
    static final String ERROR_FAILED = "failed";

    /** 新着 1 件（1 スレッド）。title・body は Go が整形済み（制御文字の除去・長さ制限。F-10）。 */
    static final class NewMail {
        final String accountId;
        final String accountEmail;
        final String threadId;
        final String title;
        final String body;

        NewMail(String accountId, String accountEmail, String threadId, String title, String body) {
            this.accountId = accountId;
            this.accountEmail = accountEmail;
            this.threadId = threadId;
            this.title = title;
            this.body = body;
        }
    }

    static final class Account {
        final String id;
        final String email;
        final String status;

        Account(String id, String email, String status) {
            this.id = id;
            this.email = email;
            this.status = status;
        }
    }

    final boolean ok;
    /** ok=false のときの理由コード（busy／failed）。ok=true なら空。 */
    final String error;
    final boolean appRunning;
    final List<NewMail> newMail;
    final List<Account> accounts;

    SyncOutcome(boolean ok, String error, boolean appRunning, List<NewMail> newMail, List<Account> accounts) {
        this.ok = ok;
        this.error = error;
        this.appRunning = appRunning;
        this.newMail = Collections.unmodifiableList(newMail);
        this.accounts = Collections.unmodifiableList(accounts);
    }

    static SyncOutcome failed(String code) {
        return new SyncOutcome(false, code, false, new ArrayList<NewMail>(), new ArrayList<Account>());
    }

    /** JSON を読む。null・空・壊れた JSON は failed の失敗。 */
    static SyncOutcome parse(String json) {
        if (json == null || json.isEmpty()) {
            return failed(ERROR_FAILED);
        }
        try {
            JSONObject o = new JSONObject(json);
            if (!o.optBoolean("ok", false)) {
                String code = o.optString("error", ERROR_FAILED);
                return failed(ERROR_BUSY.equals(code) ? ERROR_BUSY : ERROR_FAILED);
            }
            List<NewMail> mails = new ArrayList<>();
            JSONArray ms = o.optJSONArray("newMail");
            for (int i = 0; ms != null && i < ms.length(); i++) {
                JSONObject m = ms.getJSONObject(i);
                String accountId = m.optString("accountId", "");
                String threadId = m.optString("threadId", "");
                if (accountId.isEmpty() || threadId.isEmpty()) {
                    continue;
                }
                mails.add(new NewMail(accountId, m.optString("accountEmail", ""), threadId,
                        m.optString("title", ""), m.optString("body", "")));
            }
            List<Account> accounts = new ArrayList<>();
            JSONArray as = o.optJSONArray("accounts");
            for (int i = 0; as != null && i < as.length(); i++) {
                JSONObject a = as.getJSONObject(i);
                String id = a.optString("id", "");
                if (id.isEmpty()) {
                    continue;
                }
                accounts.add(new Account(id, a.optString("email", ""), a.optString("status", STATUS_ERROR)));
            }
            return new SyncOutcome(true, "", o.optBoolean("appRunning", false), mails, accounts);
        } catch (JSONException e) {
            return failed(ERROR_FAILED);
        }
    }
}
