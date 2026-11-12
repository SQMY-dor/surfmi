// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi.core

import io.github.sqmydor.surfmi.core.libcore.Libcore
import org.json.JSONArray
import org.json.JSONObject

/** One member of a policy group. delay: 0 = untested, -1 = failed, else ms. */
data class PolicyNode(val name: String, val type: String, val delay: Int, val udp: Boolean)

data class PolicyGroup(
    val name: String,
    val type: String,
    val now: String,
    val hidden: Boolean,
    val testUrl: String,
    val proxies: List<PolicyNode>,
) {
    val selectable: Boolean get() = type == "Selector"
}

data class ConnectionInfo(
    val id: String,
    val network: String,
    val host: String,
    val process: String,
    val rule: String,
    val rulePayload: String,
    val chains: List<String>,
    val upload: Long,
    val download: Long,
    val start: Long,
)

data class TrafficSnapshot(
    val up: Long = 0,
    val down: Long = 0,
    val upTotal: Long = 0,
    val downTotal: Long = 0,
)

data class ConvertResult(
    val format: String,
    val yaml: String,
    val warnings: List<String>,
    val proxies: Int,
    val groups: Int,
    val rules: Int,
    val managedUrl: String?,
    val managedInterval: Int,
)

data class SubUserinfo(val upload: Long, val download: Long, val total: Long, val expire: Long)

/** Thin, JSON-decoding facade over the gomobile [Libcore] bridge. */
object Core {
    fun version(): String = runCatching { Libcore.version() }.getOrDefault("unavailable")

    fun convert(data: ByteArray, testUrl: String, fetchRemote: Boolean): ConvertResult {
        val o = JSONObject(Libcore.convertProfile(data, testUrl, fetchRemote))
        val managed = o.optJSONObject("managed")
        return ConvertResult(
            format = o.optString("format"),
            yaml = o.optString("yaml"),
            warnings = o.optJSONArray("warnings").strings(),
            proxies = o.optInt("proxies"),
            groups = o.optInt("groups"),
            rules = o.optInt("rules"),
            managedUrl = managed?.optString("url")?.takeIf { it.isNotBlank() },
            managedInterval = managed?.optInt("interval") ?: 0,
        )
    }

    fun userinfo(header: String): SubUserinfo {
        val o = JSONObject(Libcore.parseUserinfo(header))
        return SubUserinfo(o.optLong("upload"), o.optLong("download"), o.optLong("total"), o.optLong("expire"))
    }

    fun groups(includeGlobal: Boolean): List<PolicyGroup> {
        if (!Libcore.isRunning()) return emptyList()
        val arr = JSONArray(Libcore.proxyGroups(includeGlobal))
        return (0 until arr.length()).map { i ->
            val g = arr.getJSONObject(i)
            val ps = g.optJSONArray("proxies") ?: JSONArray()
            PolicyGroup(
                name = g.optString("name"),
                type = g.optString("type"),
                now = g.optString("now"),
                hidden = g.optBoolean("hidden"),
                testUrl = g.optString("testUrl"),
                proxies = (0 until ps.length()).map { j ->
                    val p = ps.getJSONObject(j)
                    PolicyNode(p.optString("name"), p.optString("type"), p.optInt("delay"), p.optBoolean("udp"))
                },
            )
        }
    }

    fun select(group: String, name: String): Result<Unit> = runCatching { Libcore.selectProxy(group, name) }

    fun testGroup(group: String, url: String = "", timeoutMs: Int = 5000): Map<String, Int> = runCatching {
        val o = JSONObject(Libcore.testGroupDelay(group, url, timeoutMs))
        o.keys().asSequence().associateWith { o.optInt(it) }
    }.getOrDefault(emptyMap())

    fun connections(): List<ConnectionInfo> {
        if (!Libcore.isRunning()) return emptyList()
        val arr = JSONArray(Libcore.connections())
        return (0 until arr.length()).map { i ->
            val c = arr.getJSONObject(i)
            ConnectionInfo(
                id = c.optString("id"),
                network = c.optString("network"),
                host = c.optString("host"),
                process = c.optString("process"),
                rule = c.optString("rule"),
                rulePayload = c.optString("rulePayload"),
                chains = c.optJSONArray("chains").strings(),
                upload = c.optLong("upload"),
                download = c.optLong("download"),
                start = c.optLong("start"),
            )
        }
    }

    fun closeConnection(id: String) = Libcore.closeConnection(id)

    fun closeAll() = Libcore.closeAllConnections()

    fun traffic(): TrafficSnapshot {
        if (!Libcore.isRunning()) return TrafficSnapshot()
        val t = Libcore.queryTraffic()
        return TrafficSnapshot(t.getUp(), t.getDown(), t.getUpTotal(), t.getDownTotal())
    }

    fun setMode(mode: String) {
        if (Libcore.isRunning()) runCatching { Libcore.setMode(mode) }
    }

    private fun JSONArray?.strings(): List<String> =
        if (this == null) emptyList() else (0 until length()).map { optString(it) }
}
