package com.wails.app;

import android.app.ActivityManager;
import android.app.usage.UsageStatsManager;
import android.content.Context;
import android.net.ConnectivityManager;
import android.net.LinkProperties;
import android.net.Network;
import android.net.NetworkCapabilities;
import android.os.PowerManager;
import android.util.Log;

import org.json.JSONObject;

import java.net.InetAddress;
import java.util.ArrayList;
import java.util.List;

/**
 * Go（internal/androidnet）の HTTPS 通信が名前解決に失敗したときの救済と記録（#368）。Activity に依存しない（applicationContext だけを使う）。
 *
 * <ul>
 *   <li>{@link #resolve}: 有効なネットワーク（ブラウザと同じ既定のネットワーク）で {@code Network.getAllByName} により名前を引く。
 *       Go の標準の解決（bionic の getaddrinfo）が失敗したときだけ呼ばれる。DNS サーバーは端末の設定（Private DNS・VPN・社内 DNS を含む）に従う。</li>
 *   <li>{@link #diagnose}: 失敗の手がかり（アプリの前面・背面、ネットワークの種類、Private DNS、DNS サーバー、Data Saver、電池の制限、Doze、スタンバイバケット）。</li>
 * </ul>
 * 戻りは {"ok":true,"value":"…"} か {"ok":false,"error":"<コード>"}（コードは no_network・background_no_network・bad_request・unresolved・failed だけ）。
 * background_no_network は有効なネットワークが無く、アプリが背面のとき（OS が背面の通信を止めている。Go は前面へ戻るのを待って引き直す）。
 */
final class NetBridge {
    private static final String TAG = "NetBridge";

    private final Context context;

    NetBridge(Context context) {
        this.context = context.getApplicationContext() != null ? context.getApplicationContext() : context;
    }

    /** json: {"host":"…"}。value はアドレスの JSON 配列。 */
    String resolve(String json) {
        String host;
        try {
            host = new JSONObject(json).optString("host", "");
        } catch (Exception e) {
            return NetDiagnostics.fail("bad_request");
        }
        if (!NetDiagnostics.validHost(host)) return NetDiagnostics.fail("bad_request");
        try {
            ConnectivityManager cm = (ConnectivityManager) context.getSystemService(Context.CONNECTIVITY_SERVICE);
            Network network = cm == null ? null : cm.getActiveNetwork();
            if (network == null) return NetDiagnostics.fail(NetDiagnostics.noNetworkCode(myImportance()));
            List<String> out = new ArrayList<>();
            for (InetAddress a : network.getAllByName(host)) out.add(a.getHostAddress());
            if (out.isEmpty()) return NetDiagnostics.fail("unresolved");
            return NetDiagnostics.ok(NetDiagnostics.addressesJson(out));
        } catch (java.net.UnknownHostException e) {
            return NetDiagnostics.fail("unresolved");
        } catch (Exception e) {
            Log.w(TAG, "resolve failed: " + e.getClass().getName());
            return NetDiagnostics.fail("failed");
        }
    }

    /** アプリのプロセスの重要度（取れなければ前面扱いにして、待たずに失敗させる）。 */
    private static int myImportance() {
        try {
            ActivityManager.RunningAppProcessInfo info = new ActivityManager.RunningAppProcessInfo();
            ActivityManager.getMyMemoryState(info);
            return info.importance;
        } catch (Exception e) {
            return ActivityManager.RunningAppProcessInfo.IMPORTANCE_FOREGROUND;
        }
    }

    /** value は NetDiagnostics.toJson の JSON。集められない項目は空・false のまま（個々の取得の失敗で全体を落とさない）。 */
    String diagnose() {
        String process = NetDiagnostics.processName(myImportance());

        String networkType = "none", privateDns = "off", restrictBackground = "unknown";
        boolean validated = false, metered = false;
        List<String> dns = new ArrayList<>();
        try {
            ConnectivityManager cm = (ConnectivityManager) context.getSystemService(Context.CONNECTIVITY_SERVICE);
            if (cm != null) {
                restrictBackground = NetDiagnostics.restrictBackgroundName(cm.getRestrictBackgroundStatus());
                Network net = cm.getActiveNetwork();
                NetworkCapabilities caps = net == null ? null : cm.getNetworkCapabilities(net);
                if (caps != null) {
                    networkType = NetDiagnostics.networkType(true,
                            caps.hasTransport(NetworkCapabilities.TRANSPORT_VPN),
                            caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI),
                            caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR),
                            caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET));
                    validated = caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED);
                    metered = !caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_METERED);
                }
                LinkProperties lp = net == null ? null : cm.getLinkProperties(net);
                if (lp != null) {
                    privateDns = NetDiagnostics.privateDnsMode(lp.isPrivateDnsActive(), lp.getPrivateDnsServerName());
                    for (InetAddress a : lp.getDnsServers()) dns.add(a.getHostAddress());
                }
            }
        } catch (Exception e) {
            Log.w(TAG, "network state failed: " + e.getClass().getName());
        }

        boolean backgroundRestricted = false, deviceIdle = false;
        String bucket = "unknown";
        try {
            ActivityManager am = (ActivityManager) context.getSystemService(Context.ACTIVITY_SERVICE);
            backgroundRestricted = am != null && am.isBackgroundRestricted();
            PowerManager pm = (PowerManager) context.getSystemService(Context.POWER_SERVICE);
            deviceIdle = pm != null && pm.isDeviceIdleMode();
            UsageStatsManager usm = (UsageStatsManager) context.getSystemService(Context.USAGE_STATS_SERVICE);
            if (usm != null) bucket = NetDiagnostics.standbyBucketName(usm.getAppStandbyBucket());
        } catch (Exception e) {
            Log.w(TAG, "power state failed: " + e.getClass().getName());
        }
        return NetDiagnostics.ok(NetDiagnostics.toJson(process, networkType, validated, metered, privateDns, dns,
                restrictBackground, backgroundRestricted, deviceIdle, bucket));
    }
}
