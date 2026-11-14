package com.wails.app;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.json.JSONArray;
import org.json.JSONObject;
import org.junit.Test;

import java.util.Arrays;
import java.util.Collections;

public class NetDiagnosticsTest {
    @Test
    public void processNameDistinguishesForegroundFromCached() {
        assertEquals("foreground", NetDiagnostics.processName(100));
        assertEquals("visible", NetDiagnostics.processName(200));
        assertEquals("service", NetDiagnostics.processName(300));
        assertEquals("cached", NetDiagnostics.processName(400));
        assertEquals("cached", NetDiagnostics.processName(1000));
    }

    @Test
    public void noNetworkCodeAsksGoToWaitOnlyWhenInBackground() {
        assertEquals("no_network", NetDiagnostics.noNetworkCode(100));
        assertEquals("no_network", NetDiagnostics.noNetworkCode(125));
        assertEquals("no_network", NetDiagnostics.noNetworkCode(200));
        assertEquals("background_no_network", NetDiagnostics.noNetworkCode(230));
        assertEquals("background_no_network", NetDiagnostics.noNetworkCode(300));
        assertEquals("background_no_network", NetDiagnostics.noNetworkCode(400));
    }

    @Test
    public void networkTypePrefersVpn() {
        assertEquals("none", NetDiagnostics.networkType(false, false, true, false, false));
        assertEquals("vpn", NetDiagnostics.networkType(true, true, true, false, false));
        assertEquals("wifi", NetDiagnostics.networkType(true, false, true, false, false));
        assertEquals("cellular", NetDiagnostics.networkType(true, false, false, true, false));
        assertEquals("ethernet", NetDiagnostics.networkType(true, false, false, false, true));
        assertEquals("other", NetDiagnostics.networkType(true, false, false, false, false));
    }

    @Test
    public void privateDnsModeDoesNotNeedTheHostName() {
        assertEquals("strict", NetDiagnostics.privateDnsMode(true, "dns.example"));
        assertEquals("opportunistic", NetDiagnostics.privateDnsMode(true, null));
        assertEquals("opportunistic", NetDiagnostics.privateDnsMode(true, ""));
        assertEquals("off", NetDiagnostics.privateDnsMode(false, null));
    }

    @Test
    public void dataSaverAndStandbyBucketNames() {
        assertEquals("disabled", NetDiagnostics.restrictBackgroundName(1));
        assertEquals("whitelisted", NetDiagnostics.restrictBackgroundName(2));
        assertEquals("enabled", NetDiagnostics.restrictBackgroundName(3));
        assertEquals("unknown", NetDiagnostics.restrictBackgroundName(0));
        assertEquals("active", NetDiagnostics.standbyBucketName(10));
        assertEquals("working_set", NetDiagnostics.standbyBucketName(20));
        assertEquals("frequent", NetDiagnostics.standbyBucketName(30));
        assertEquals("rare", NetDiagnostics.standbyBucketName(40));
        assertEquals("restricted", NetDiagnostics.standbyBucketName(45));
    }

    @Test
    public void toJsonCarriesTheFieldsGoReads() throws Exception {
        String json = NetDiagnostics.toJson("cached", "wifi", true, false, "strict",
                Arrays.asList("192.0.2.1", "2001:db8::1"), "enabled", true, false, "rare");
        JSONObject o = new JSONObject(json);
        assertEquals("cached", o.getString("process"));
        assertEquals("wifi", o.getString("network"));
        assertTrue(o.getBoolean("validated"));
        assertFalse(o.getBoolean("metered"));
        assertEquals("strict", o.getString("private_dns"));
        assertEquals("enabled", o.getString("restrict_background"));
        assertTrue(o.getBoolean("background_restricted"));
        assertFalse(o.getBoolean("device_idle"));
        assertEquals("rare", o.getString("standby_bucket"));
        JSONArray servers = o.getJSONArray("dns_servers");
        assertEquals(2, servers.length());
        assertEquals("192.0.2.1", servers.getString(0));
    }

    @Test
    public void envelopesAreOkOrCodeOnly() throws Exception {
        JSONObject ok = new JSONObject(NetDiagnostics.ok(NetDiagnostics.addressesJson(Arrays.asList("192.0.2.1"))));
        assertTrue(ok.getBoolean("ok"));
        assertEquals("[\"192.0.2.1\"]", ok.getString("value"));
        JSONObject fail = new JSONObject(NetDiagnostics.fail("unresolved"));
        assertFalse(fail.getBoolean("ok"));
        assertEquals("unresolved", fail.getString("error"));
        assertEquals("[]", NetDiagnostics.addressesJson(Collections.<String>emptyList()));
    }

    @Test
    public void validHostRejectsNonHostnames() {
        assertTrue(NetDiagnostics.validHost("auth.openai.com"));
        assertTrue(NetDiagnostics.validHost("oauth2.googleapis.com"));
        assertFalse(NetDiagnostics.validHost(null));
        assertFalse(NetDiagnostics.validHost(""));
        assertFalse(NetDiagnostics.validHost("a b"));
        assertFalse(NetDiagnostics.validHost("host/path"));
        assertFalse(NetDiagnostics.validHost("host?token=x"));
        assertFalse(NetDiagnostics.validHost(new String(new char[254]).replace('\0', 'a')));
    }
}
