package com.wails.app;

import static org.junit.Assert.assertNotEquals;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

/** レンダラが消えた理由のログ文言: クラッシュと OS による終了（メモリ不足）で文言を分ける。 */
public class RenderGoneReasonTest {
    @Test
    public void crashAndSystemKillHaveDistinctMessages() {
        String crash = RenderGoneReason.describe(true);
        String killed = RenderGoneReason.describe(false);
        assertNotEquals(crash, killed);
        assertTrue(crash.contains("crashed"));
        assertTrue(killed.contains("killed") && killed.contains("low memory"));
    }
}
