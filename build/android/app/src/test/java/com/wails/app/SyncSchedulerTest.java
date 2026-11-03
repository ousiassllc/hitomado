package com.wails.app;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertTrue;

import androidx.work.NetworkType;
import androidx.work.PeriodicWorkRequest;

import org.junit.Test;

import java.util.concurrent.TimeUnit;

/** 定期同期の要求（SyncScheduler.request）の中身: 15 分間隔・ネットワーク接続が条件・Worker は SyncWorker。 */
public class SyncSchedulerTest {
    @Test
    public void requestIsFifteenMinutePeriodicOnConnectedNetwork() {
        PeriodicWorkRequest r = SyncScheduler.request();
        assertEquals(TimeUnit.MINUTES.toMillis(15), r.getWorkSpec().intervalDuration);
        assertEquals(NetworkType.CONNECTED, r.getWorkSpec().constraints.getRequiredNetworkType());
        assertEquals(SyncWorker.class.getName(), r.getWorkSpec().workerClassName);
        assertTrue(r.getWorkSpec().isPeriodic());
    }

    @Test
    public void uniqueNameIsStable() {
        assertEquals("hitomado-sync", SyncScheduler.WORK_NAME);
    }
}
