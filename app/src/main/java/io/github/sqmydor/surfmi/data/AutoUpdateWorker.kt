// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi.data

import android.content.Context
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.NetworkType
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import java.util.concurrent.TimeUnit

/** Hourly check that refreshes subscriptions whose interval has elapsed. */
class AutoUpdateWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        var failed = false
        for (p in ProfileStore.dueForUpdate()) {
            runCatching { ProfileStore.update(p) }.onFailure { failed = true }
        }
        return if (failed) Result.retry() else Result.success()
    }

    companion object {
        private const val NAME = "profile-auto-update"

        fun schedule(context: Context) {
            val request = PeriodicWorkRequestBuilder<AutoUpdateWorker>(1, TimeUnit.HOURS)
                .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())
                .build()
            WorkManager.getInstance(context)
                .enqueueUniquePeriodicWork(NAME, ExistingPeriodicWorkPolicy.KEEP, request)
        }
    }
}
