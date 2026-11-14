package com.wails.app;

import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;

import java.util.List;

/**
 * 名前解決の失敗（#368）のとき Go がログに残す、端末のネットワークの状態の JSON と、名前解決の結果の封筒。
 * Android のクラスに依存しない（値は呼び出し側 NetBridge が集めて渡す）ので、JVM で単体テストする。
 * 載せるのは種類・状態・端末の DNS サーバーのアドレスだけ。Private DNS のホスト名（利用者の識別子を含みうる）・
 * SSID・アカウント・URL は載せない。
 */
final class NetDiagnostics {
    private NetDiagnostics() {}

    // ActivityManager.RunningAppProcessInfo.IMPORTANCE_*。
    private static final int IMPORTANCE_FOREGROUND = 100;
    private static final int IMPORTANCE_FOREGROUND_SERVICE = 125;
    private static final int IMPORTANCE_TOP_SLEEPING = 150;
    private static final int IMPORTANCE_VISIBLE = 200;
    private static final int IMPORTANCE_PERCEPTIBLE = 230;
    private static final int IMPORTANCE_SERVICE = 300;
    private static final int IMPORTANCE_CANT_SAVE_STATE = 350;

    /** アプリのプロセスの重要度。foreground・visible・service は前面または前面に準ずる、cached は背面（OS が通信を止めうる）。 */
    static String processName(int importance) {
        if (importance <= IMPORTANCE_FOREGROUND) return "foreground";
        if (importance <= IMPORTANCE_FOREGROUND_SERVICE) return "foreground_service";
        if (importance <= IMPORTANCE_TOP_SLEEPING) return "top_sleeping";
        if (importance <= IMPORTANCE_VISIBLE) return "visible";
        if (importance <= IMPORTANCE_PERCEPTIBLE) return "perceptible";
        if (importance <= IMPORTANCE_SERVICE) return "service";
        if (importance <= IMPORTANCE_CANT_SAVE_STATE) return "cant_save_state";
        return "cached";
    }

    /**
     * 有効なネットワークが無いときの失敗コード。アプリが背面（visible より下）なら、OS が背面の通信を止めている（Data Saver・
     * 電池の制限・Doze など）可能性があり、前面に戻れば通るので Go に待たせる（background_no_network）。前面なら本当に通信が無い（no_network）。
     */
    static String noNetworkCode(int importance) {
        return importance > IMPORTANCE_VISIBLE ? "background_no_network" : "no_network";
    }

    /** ネットワークの種類。VPN は下の物理ネットワークより優先して示す（通信が VPN を通るため）。 */
    static String networkType(boolean hasNetwork, boolean vpn, boolean wifi, boolean cellular, boolean ethernet) {
        if (!hasNetwork) return "none";
        if (vpn) return "vpn";
        if (wifi) return "wifi";
        if (cellular) return "cellular";
        if (ethernet) return "ethernet";
        return "other";
    }

    /** Private DNS の状態: strict（ホスト名指定）・opportunistic（自動で有効）・off。 */
    static String privateDnsMode(boolean active, String serverName) {
        if (serverName != null && !serverName.isEmpty()) return "strict";
        return active ? "opportunistic" : "off";
    }

    /** ConnectivityManager.getRestrictBackgroundStatus() の値（Data Saver）。 */
    static String restrictBackgroundName(int status) {
        switch (status) {
            case 1: return "disabled";
            case 2: return "whitelisted";
            case 3: return "enabled";
            default: return "unknown";
        }
    }

    /** UsageStatsManager.getAppStandbyBucket() の値。 */
    static String standbyBucketName(int bucket) {
        if (bucket <= 5) return "exempted";
        if (bucket <= 10) return "active";
        if (bucket <= 20) return "working_set";
        if (bucket <= 30) return "frequent";
        if (bucket <= 40) return "rare";
        if (bucket <= 45) return "restricted";
        return "never";
    }

    /** Go（internal/androidnet.Diagnostics）が読む JSON。 */
    static String toJson(String process, String network, boolean validated, boolean metered, String privateDns,
                         List<String> dnsServers, String restrictBackground, boolean backgroundRestricted,
                         boolean deviceIdle, String standbyBucket) {
        try {
            JSONArray servers = new JSONArray();
            for (String s : dnsServers) servers.put(s);
            return new JSONObject()
                    .put("process", process)
                    .put("network", network)
                    .put("validated", validated)
                    .put("metered", metered)
                    .put("private_dns", privateDns)
                    .put("dns_servers", servers)
                    .put("restrict_background", restrictBackground)
                    .put("background_restricted", backgroundRestricted)
                    .put("device_idle", deviceIdle)
                    .put("standby_bucket", standbyBucket)
                    .toString();
        } catch (JSONException e) {
            return "{}";
        }
    }

    /** 成功の封筒。value は JSON の文字列（値自体が JSON）。 */
    static String ok(String value) {
        try {
            return new JSONObject().put("ok", true).put("value", value).toString();
        } catch (JSONException e) {
            return fail("failed");
        }
    }

    /** 失敗の封筒。code はコードだけ（例外メッセージ・ホスト名は含めない）。 */
    static String fail(String code) {
        return "{\"ok\":false,\"error\":\"" + code + "\"}";
    }

    /** 解決したアドレス（IP のリテラル）の JSON 配列。 */
    static String addressesJson(List<String> addresses) {
        JSONArray a = new JSONArray();
        for (String s : addresses) a.put(s);
        return a.toString();
    }

    /**
     * 解決する名前の検査。空・長すぎる・英数字とハイフンとドット以外を含むものは拒否する（Go の呼び出しの取り違いを弾く）。
     * 数字とドットだけの {@code 1.2.3.4} は通る（IPv4 リテラルは弾かない。{@code :} を含む IPv6 リテラルは拒否される）。
     */
    static boolean validHost(String host) {
        if (host == null || host.isEmpty() || host.length() > 253) return false;
        for (int i = 0; i < host.length(); i++) {
            char c = host.charAt(i);
            boolean ok = (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '.';
            if (!ok) return false;
        }
        return true;
    }
}
