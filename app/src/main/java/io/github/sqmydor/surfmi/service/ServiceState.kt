// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi.service

import io.github.sqmydor.surfmi.core.TrafficSnapshot
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

enum class VpnState { Stopped, Starting, Running, Stopping }

data class LogLine(val time: Long, val level: String, val message: String)

/** Process-wide observable state shared by the service, tile and UI. */
object ServiceState {
    private val _state = MutableStateFlow(VpnState.Stopped)
    val state: StateFlow<VpnState> = _state.asStateFlow()

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error.asStateFlow()

    private val _traffic = MutableStateFlow(TrafficSnapshot())
    val traffic: StateFlow<TrafficSnapshot> = _traffic.asStateFlow()

    private val _startedAt = MutableStateFlow(0L)
    val startedAt: StateFlow<Long> = _startedAt.asStateFlow()

    fun set(s: VpnState) {
        _state.value = s
        if (s == VpnState.Running) _startedAt.value = System.currentTimeMillis()
        if (s == VpnState.Stopped) _traffic.value = TrafficSnapshot()
    }

    fun setError(msg: String?) {
        _error.value = msg
    }

    fun setTraffic(t: TrafficSnapshot) {
        _traffic.value = t
    }
}

/** Bounded in-memory ring buffer of core log lines. */
object LogBuffer {
    private const val CAPACITY = 2000
    private val lines = ArrayDeque<LogLine>(CAPACITY)
    private val _version = MutableStateFlow(0L)
    /** Bumped on every change; collect it and call [snapshot]. */
    val version: StateFlow<Long> = _version.asStateFlow()

    @Synchronized
    fun add(level: String, message: String) {
        if (lines.size >= CAPACITY) lines.removeFirst()
        lines.addLast(LogLine(System.currentTimeMillis(), level, message))
        _version.value++
    }

    @Synchronized
    fun snapshot(): List<LogLine> = lines.toList()

    @Synchronized
    fun clear() {
        lines.clear()
        _version.value++
    }
}
