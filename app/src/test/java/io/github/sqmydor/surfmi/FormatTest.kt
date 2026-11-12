// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi

import io.github.sqmydor.surfmi.util.formatBytes
import io.github.sqmydor.surfmi.util.formatDuration
import io.github.sqmydor.surfmi.util.formatSpeed
import org.junit.Assert.assertEquals
import org.junit.Test

class FormatTest {
    @Test
    fun bytes() {
        assertEquals("0 B", formatBytes(0))
        assertEquals("1023 B", formatBytes(1023))
        assertEquals("1.00 KB", formatBytes(1024))
        assertEquals("1.50 MB", formatBytes(1536L * 1024))
        assertEquals("10.0 GB", formatBytes(10L * 1024 * 1024 * 1024))
        assertEquals("200 MB/s", formatSpeed(200L * 1024 * 1024))
    }

    @Test
    fun duration() {
        assertEquals("0:00:59", formatDuration(59_000))
        assertEquals("1:01:01", formatDuration(3_661_000))
    }
}
