// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi.data

import android.content.Context
import android.util.Base64
import io.github.sqmydor.surfmi.core.Core
import io.github.sqmydor.surfmi.core.libcore.Libcore
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.withContext
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.net.HttpURLConnection
import java.net.URL
import java.net.URLDecoder
import java.util.UUID

data class Profile(
    val id: String = UUID.randomUUID().toString(),
    val name: String,
    /** "url" for subscriptions, "local" for imported files / pasted text. */
    val type: String,
    val url: String = "",
    val userAgent: String = "",
    /** Auto-update interval in hours; 0 disables it. */
    val updateHours: Int = 24,
    val lastUpdated: Long = 0,
    val upload: Long = 0,
    val download: Long = 0,
    val total: Long = 0,
    val expire: Long = 0,
    val format: String = "",
    val proxies: Int = 0,
    val groups: Int = 0,
    val rules: Int = 0,
    val warnings: List<String> = emptyList(),
) {
    val isRemote: Boolean get() = type == "url"

    fun toJson(): JSONObject = JSONObject()
        .put("id", id).put("name", name).put("type", type).put("url", url).put("userAgent", userAgent)
        .put("updateHours", updateHours).put("lastUpdated", lastUpdated)
        .put("upload", upload).put("download", download).put("total", total).put("expire", expire)
        .put("format", format).put("proxies", proxies).put("groups", groups).put("rules", rules)
        .put("warnings", JSONArray(warnings))

    companion object {
        fun fromJson(o: JSONObject) = Profile(
            id = o.getString("id"),
            name = o.optString("name"),
            type = o.optString("type", "local"),
            url = o.optString("url"),
            userAgent = o.optString("userAgent"),
            updateHours = o.optInt("updateHours", 24),
            lastUpdated = o.optLong("lastUpdated"),
            upload = o.optLong("upload"),
            download = o.optLong("download"),
            total = o.optLong("total"),
            expire = o.optLong("expire"),
            format = o.optString("format"),
            proxies = o.optInt("proxies"),
            groups = o.optInt("groups"),
            rules = o.optInt("rules"),
            warnings = o.optJSONArray("warnings")?.let { a -> (0 until a.length()).map { a.optString(it) } } ?: emptyList(),
        )
    }
}

/** Downloaded subscription body plus the headers we care about. */
private class Download(val body: ByteArray, val userinfo: String?, val title: String?, val updateHours: Int?)

/**
 * Stores profiles under filesDir/profiles/<id>/ (source = original content,
 * config.yaml = converted mihomo config) and their metadata in profiles.json.
 */
object ProfileStore {
    private lateinit var root: File
    private val state = MutableStateFlow<List<Profile>>(emptyList())
    val profiles: StateFlow<List<Profile>> = state.asStateFlow()

    /** mihomo home: geo databases, rule/proxy providers, cache.db. */
    lateinit var homeDir: File
        private set

    fun init(context: Context) {
        root = File(context.filesDir, "profiles").apply { mkdirs() }
        homeDir = File(context.filesDir, "mihomo").apply { mkdirs() }
        state.value = load()
    }

    private val indexFile get() = File(root, "profiles.json")

    private fun load(): List<Profile> = runCatching {
        val arr = JSONArray(indexFile.readText())
        (0 until arr.length()).map { Profile.fromJson(arr.getJSONObject(it)) }
    }.getOrDefault(emptyList())

    @Synchronized
    private fun persist(list: List<Profile>) {
        val arr = JSONArray()
        list.forEach { arr.put(it.toJson()) }
        val tmp = File(root, "profiles.json.tmp")
        tmp.writeText(arr.toString())
        tmp.renameTo(indexFile)
        state.value = list
    }

    fun dir(p: Profile) = File(root, p.id).apply { mkdirs() }
    fun configFile(p: Profile) = File(dir(p), "config.yaml")
    fun sourceFile(p: Profile) = File(dir(p), "source")

    fun get(id: String): Profile? = state.value.firstOrNull { it.id == id }

    fun active(): Profile? = get(Settings.activeProfile.value) ?: state.value.firstOrNull()

    fun setActive(p: Profile) {
        Settings.activeProfile.value = p.id
    }

    private fun upsert(p: Profile) {
        val list = state.value.toMutableList()
        val i = list.indexOfFirst { it.id == p.id }
        if (i >= 0) list[i] = p else list.add(p)
        persist(list)
        if (Settings.activeProfile.value.isEmpty()) setActive(p)
    }

    fun delete(p: Profile) {
        dir(p).deleteRecursively()
        persist(state.value.filterNot { it.id == p.id })
        if (Settings.activeProfile.value == p.id) {
            Settings.activeProfile.value = state.value.firstOrNull()?.id ?: ""
        }
    }

    fun rename(p: Profile, name: String, url: String, updateHours: Int, userAgent: String) =
        upsert(p.copy(name = name, url = url, updateHours = updateHours, userAgent = userAgent))

    /** Adds a subscription URL, downloading and converting it. */
    suspend fun addRemote(name: String, url: String, updateHours: Int, userAgent: String): Profile {
        val p = Profile(name = name, type = "url", url = url.trim(), updateHours = updateHours, userAgent = userAgent)
        return update(p, isNew = true)
    }

    /** Adds local content (file import or pasted text). */
    suspend fun addLocal(name: String, content: ByteArray): Profile = withContext(Dispatchers.IO) {
        val base = Profile(name = name, type = "local", updateHours = 0)
        applyContent(base, content, null)
    }

    /** Replaces the source of a profile (editor) and re-converts it. */
    suspend fun saveSource(p: Profile, content: String): Profile = withContext(Dispatchers.IO) {
        applyContent(p, content.toByteArray(), null)
    }

    fun readSource(p: Profile): String = runCatching { sourceFile(p).readText() }.getOrDefault("")
    fun readConfig(p: Profile): String = runCatching { configFile(p).readText() }.getOrDefault("")

    /** Re-downloads a remote profile. Throws on network/convert errors. */
    suspend fun update(p: Profile, isNew: Boolean = false): Profile = withContext(Dispatchers.IO) {
        require(p.isRemote) { "not a remote profile" }
        val dl = download(p.url, p.userAgent.ifBlank { Settings.userAgent.value })
        var named = p
        if (isNew && p.name.isBlank()) named = p.copy(name = dl.title ?: hostOf(p.url))
        if (dl.updateHours != null && dl.updateHours > 0 && isNew) named = named.copy(updateHours = dl.updateHours)
        val updated = applyContent(named, dl.body, dl.userinfo)
        reloadIfActive(updated)
        updated
    }

    private fun applyContent(p: Profile, content: ByteArray, userinfo: String?): Profile {
        val result = Core.convert(content, Settings.testUrl.value, fetchRemote = true)
        sourceFile(p).writeBytes(content)
        configFile(p).writeText(result.yaml)
        var np = p.copy(
            lastUpdated = System.currentTimeMillis(),
            format = result.format,
            proxies = result.proxies,
            groups = result.groups,
            rules = result.rules,
            warnings = result.warnings,
        )
        if (np.name.isBlank()) np = np.copy(name = "Profile ${state.value.size + 1}")
        if (np.isRemote && np.updateHours == 0 && result.managedInterval > 0) {
            np = np.copy(updateHours = maxOf(1, result.managedInterval / 3600))
        }
        if (userinfo != null) {
            val ui = Core.userinfo(userinfo)
            np = np.copy(upload = ui.upload, download = ui.download, total = ui.total, expire = ui.expire)
        }
        upsert(np)
        return np
    }

    private fun reloadIfActive(p: Profile) {
        if (active()?.id == p.id && Libcore.isRunning()) {
            runCatching { Libcore.reload(configFile(p).absolutePath) }
        }
    }

    private fun hostOf(url: String) = runCatching { URL(url).host }.getOrDefault("Subscription")

    private fun download(url: String, ua: String): Download {
        var target = URL(url)
        repeat(5) {
            val conn = (target.openConnection() as HttpURLConnection).apply {
                connectTimeout = 15_000
                readTimeout = 30_000
                instanceFollowRedirects = true
                setRequestProperty("User-Agent", ua)
                setRequestProperty("Accept", "*/*")
            }
            try {
                val code = conn.responseCode
                if (code in 300..399) {
                    val loc = conn.getHeaderField("Location") ?: error("redirect without location")
                    target = URL(target, loc)
                    return@repeat
                }
                if (code !in 200..299) error("HTTP $code")
                val body = conn.inputStream.use { it.readBytes() }
                return Download(
                    body = body,
                    userinfo = conn.getHeaderField("Subscription-Userinfo"),
                    title = parseTitle(conn.getHeaderField("profile-title"), conn.getHeaderField("Content-Disposition")),
                    updateHours = conn.getHeaderField("profile-update-interval")?.trim()?.toIntOrNull(),
                )
            } finally {
                conn.disconnect()
            }
        }
        error("too many redirects")
    }

    private fun parseTitle(profileTitle: String?, disposition: String?): String? {
        if (!profileTitle.isNullOrBlank()) {
            if (profileTitle.startsWith("base64:")) {
                return runCatching { String(Base64.decode(profileTitle.removePrefix("base64:"), Base64.DEFAULT)) }.getOrNull()
            }
            return profileTitle
        }
        if (disposition.isNullOrBlank()) return null
        Regex("filename\\*=(?:UTF-8'')?([^;]+)", RegexOption.IGNORE_CASE).find(disposition)?.let {
            return runCatching { URLDecoder.decode(it.groupValues[1].trim('"'), "UTF-8") }.getOrNull()
        }
        Regex("filename=\"?([^\";]+)\"?", RegexOption.IGNORE_CASE).find(disposition)?.let {
            return it.groupValues[1]
        }
        return null
    }

    /** Profiles whose auto-update interval has elapsed. */
    fun dueForUpdate(now: Long = System.currentTimeMillis()): List<Profile> = state.value.filter {
        it.isRemote && it.updateHours > 0 && now - it.lastUpdated >= it.updateHours * 3_600_000L
    }
}
