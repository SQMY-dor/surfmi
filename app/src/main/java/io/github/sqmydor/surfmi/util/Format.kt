// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi.util

import java.text.DateFormat
import java.util.Date
import java.util.Locale

private val units = arrayOf("B", "KB", "MB", "GB", "TB", "PB")

/** 1536 -> "1.50 KB" */
fun formatBytes(bytes: Long): String {
    if (bytes < 1024) return "$bytes B"
    var v = bytes.toDouble()
    var i = 0
    while (v >= 1024 && i < units.lastIndex) {
        v /= 1024
        i++
    }
    return String.format(Locale.ROOT, if (v >= 100) "%.0f %s" else if (v >= 10) "%.1f %s" else "%.2f %s", v, units[i])
}

fun formatSpeed(bytesPerSecond: Long): String = formatBytes(bytesPerSecond) + "/s"

fun formatDate(epochMillis: Long): String =
    if (epochMillis <= 0) "-" else DateFormat.getDateTimeInstance(DateFormat.SHORT, DateFormat.SHORT).format(Date(epochMillis))

fun formatDay(epochSeconds: Long): String =
    if (epochSeconds <= 0) "-" else DateFormat.getDateInstance(DateFormat.MEDIUM).format(Date(epochSeconds * 1000))

/** Elapsed time "1:02:03" */
fun formatDuration(millis: Long): String {
    val s = (millis / 1000).coerceAtLeast(0)
    return String.format(Locale.ROOT, "%d:%02d:%02d", s / 3600, (s / 60) % 60, s % 60)
}
