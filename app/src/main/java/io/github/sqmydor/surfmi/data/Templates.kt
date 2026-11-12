// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi.data

import android.content.Context
import io.github.sqmydor.surfmi.core.Core
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
import java.security.MessageDigest
import java.util.Locale
import java.util.UUID

/**
 * A local rule template ("规则组替换"): proxy groups + rules that replace (or
 * extend) the ones in a subscription. Built-in templates come from the Go
 * core; user templates live under filesDir/templates/.
 */
data class RuleTemplate(
    val id: String,
    val name: String,
    val description: String = "",
    val builtin: Boolean = false,
    val content: String = "",
    val sourceUrl: String = "",
    val updated: Long = 0,
)

/** How a template is merged into a subscription (matches the Go core). */
object TemplateMode {
    const val REPLACE = "replace"
    const val APPEND = "append"
    const val PREPEND = "prepend"
    val ALL = listOf(REPLACE, APPEND, PREPEND)
}

object TemplateStore {
    private lateinit var root: File
    private val state = MutableStateFlow<List<RuleTemplate>>(emptyList())
    val templates: StateFlow<List<RuleTemplate>> = state.asStateFlow()
    private var builtins: List<RuleTemplate> = emptyList()

    fun init(context: Context) {
        root = File(context.filesDir, "templates").apply { mkdirs() }
        refresh()
    }

    private val indexFile get() = File(root, "templates.json")
    private fun contentFile(id: String) = File(root, "${id.replace(Regex("[^A-Za-z0-9_-]"), "_")}.txt")

    private fun loadBuiltins(): List<RuleTemplate> = runCatching {
        val zh = Locale.getDefault().language == "zh"
        val arr = JSONArray(Core.builtinTemplates())
        (0 until arr.length()).map { i ->
            val o = arr.getJSONObject(i)
            RuleTemplate(
                id = o.optString("id"),
                name = if (zh) o.optString("name") else o.optString("nameEn", o.optString("name")),
                description = if (zh) o.optString("description") else o.optString("descriptionEn"),
                builtin = true,
                content = o.optString("content"),
            )
        }
    }.getOrDefault(emptyList())

    private fun loadUser(): List<RuleTemplate> = runCatching {
        val arr = JSONArray(indexFile.readText())
        (0 until arr.length()).map { i ->
            val o = arr.getJSONObject(i)
            val id = o.getString("id")
            RuleTemplate(
                id = id,
                name = o.optString("name"),
                sourceUrl = o.optString("sourceUrl"),
                updated = o.optLong("updated"),
                content = runCatching { contentFile(id).readText() }.getOrDefault(""),
            )
        }
    }.getOrDefault(emptyList())

    private fun refresh() {
        if (builtins.isEmpty()) builtins = loadBuiltins()
        state.value = builtins + loadUser()
    }

    @Synchronized
    private fun persist(user: List<RuleTemplate>) {
        val arr = JSONArray()
        user.forEach {
            arr.put(JSONObject().put("id", it.id).put("name", it.name).put("sourceUrl", it.sourceUrl).put("updated", it.updated))
        }
        val tmp = File(root, "templates.json.tmp")
        tmp.writeText(arr.toString())
        tmp.renameTo(indexFile)
        state.value = builtins + user
    }

    fun get(id: String): RuleTemplate? = if (id.isBlank()) null else state.value.firstOrNull { it.id == id }

    /** Returns an error message, or null when [content] is a valid template. */
    fun validate(content: String): String? = Core.validateTemplate(content)

    /** Creates or updates a user template; throws if the content is invalid. */
    fun save(id: String?, name: String, content: String, sourceUrl: String = ""): RuleTemplate {
        validate(content)?.let { error(it) }
        val user = state.value.filterNot { it.builtin }.toMutableList()
        val existing = id?.let { i -> user.indexOfFirst { it.id == i } } ?: -1
        val t = RuleTemplate(
            id = if (existing >= 0) user[existing].id else "user:" + UUID.randomUUID().toString(),
            name = name.ifBlank { "Template ${user.size + 1}" },
            content = content,
            sourceUrl = sourceUrl.ifBlank { if (existing >= 0) user[existing].sourceUrl else "" },
            updated = System.currentTimeMillis(),
        )
        contentFile(t.id).writeText(content)
        if (existing >= 0) user[existing] = t else user.add(t)
        persist(user)
        return t
    }

    fun delete(id: String) {
        val user = state.value.filterNot { it.builtin }
        if (user.none { it.id == id }) return
        contentFile(id).delete()
        persist(user.filterNot { it.id == id })
    }

    /** Downloads a template from [url] and stores it as a new user template. */
    suspend fun importUrl(name: String, url: String): RuleTemplate = withContext(Dispatchers.IO) {
        val conn = (URL(url.trim()).openConnection() as HttpURLConnection).apply {
            connectTimeout = 15_000
            readTimeout = 30_000
            instanceFollowRedirects = true
            setRequestProperty("User-Agent", Settings.userAgent.value)
        }
        val body = try {
            if (conn.responseCode !in 200..299) error("HTTP ${conn.responseCode}")
            conn.inputStream.use { it.readBytes() }.toString(Charsets.UTF_8)
        } finally {
            conn.disconnect()
        }
        val fallback = url.substringAfterLast('/').substringBefore('?').substringBeforeLast('.').ifBlank { "Imported" }
        save(null, name.ifBlank { fallback }, body, sourceUrl = url.trim())
    }

    /**
     * Fingerprint of what a profile's template selection currently resolves
     * to; a profile whose stored fingerprint differs must be re-converted.
     */
    fun signature(templateId: String, mode: String): String {
        val t = get(templateId) ?: return ""
        val md = MessageDigest.getInstance("SHA-256")
        md.update("$templateId|$mode|".toByteArray())
        md.update(t.content.toByteArray())
        return md.digest().joinToString("") { "%02x".format(it) }.take(16)
    }
}
