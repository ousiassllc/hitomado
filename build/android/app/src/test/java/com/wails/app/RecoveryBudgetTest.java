package com.wails.app;

import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

/** 再試行の上限（RecoveryBudget）: 窓の中で M 回まで、窓を過ぎた古い試行は数えない。 */
public class RecoveryBudgetTest {
    private static final long MIN = 60_000L;

    @Test
    public void allowsUpToMaxAttemptsInTheWindow() {
        RecoveryBudget b = new RecoveryBudget(3, 5 * MIN);
        assertTrue(b.tryAcquire(0));
        assertTrue(b.tryAcquire(1_000));
        assertTrue(b.tryAcquire(2_000));
        assertFalse("4th within the window is refused", b.tryAcquire(3_000));
    }

    @Test
    public void refusedAttemptsAreNotRecorded() {
        RecoveryBudget b = new RecoveryBudget(1, MIN);
        assertTrue(b.tryAcquire(0));
        assertFalse(b.tryAcquire(10));
        assertFalse(b.tryAcquire(20));
        // 拒否は数えないので、最初の 1 回が窓を出た時点で再び許可される
        assertTrue(b.tryAcquire(MIN));
    }

    @Test
    public void oldAttemptsLeaveTheWindow() {
        RecoveryBudget b = new RecoveryBudget(2, 5 * MIN);
        assertTrue(b.tryAcquire(0));
        assertTrue(b.tryAcquire(1 * MIN));
        assertFalse(b.tryAcquire(2 * MIN));
        // 0 の試行が窓（5 分）から出る
        assertTrue(b.tryAcquire(5 * MIN));
        assertFalse(b.tryAcquire(5 * MIN + 1));
        // 1 分の試行も出る
        assertTrue(b.tryAcquire(6 * MIN));
    }

    @Test
    public void attemptsSpacedBeyondTheWindowNeverExhaust() {
        RecoveryBudget b = new RecoveryBudget(1, MIN);
        for (int i = 0; i < 100; i++) {
            assertTrue(b.tryAcquire(i * MIN));
        }
    }

    @Test(expected = IllegalArgumentException.class)
    public void rejectsNonPositiveMax() {
        new RecoveryBudget(0, MIN);
    }

    @Test(expected = IllegalArgumentException.class)
    public void rejectsNonPositiveWindow() {
        new RecoveryBudget(1, 0);
    }
}
