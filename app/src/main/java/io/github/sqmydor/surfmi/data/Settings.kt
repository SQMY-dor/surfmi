// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi.data

import android.content.Context
import android.content.SharedPreferences
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/** A SharedPreferences-backed value observable from Compose. */
class Pref<T>(
    private val prefs: () -> SharedPreferences,
    private val key: String,
    private val default: T,
    private val read: SharedPreferences.(String, T) -> T,
    private val write: SharedPreferences.Editor.(String, T) -> Unit,
) {
    private val state by lazy { MutableStateFlow(prefs().read(key, default)) }
    val flow: StateFlow<T> get() = state.asStateFlow()
    var value: T
        get() = state.value
        set(v) {
            prefs().edit().apply { write(key, v) }.apply()
            state.value = v
        }
}

object PerAppMode {
    const val OFF = "off"
    const val ALLOW = "allow" // only listed apps use the VPN
    const val BYPASS = "bypass" // listed apps skip the VPN
}

object ThemeMode {
    const val SYSTEM = 0
    const val LIGHT = 1
    const val DARK = 2
    const val MONET = 3
}

object Settings {
    private lateinit var sp: SharedPreferences
    private val p = { sp }

    fun init(context: Context) {
        sp = context.getSharedPreferences("settings", Context.MODE_PRIVATE)
    }

    private fun str(key: String, def: String) = Pref(p, key, def, { k, d -> getString(k, d) ?: d }, { k, v -> putString(k, v) })
    private fun bool(key: String, def: Boolean) = Pref(p, key, def, { k, d -> getBoolean(k, d) }, { k, v -> putBoolean(k, v) })
    private fun int(key: String, def: Int) = Pref(p, key, def, { k, d -> getInt(k, d) }, { k, v -> putInt(k, v) })
    private fun strSet(key: String) = Pref<Set<String>>(p, key, emptySet(), { k, d -> getStringSet(k, d)?.toSet() ?: d }, { k, v -> putStringSet(k, v) })

    val activeProfile = str("active_profile", "")
    val mode = str("mode", "rule")
    val stack = str("tun_stack", "gvisor")
    val ipv6 = bool("ipv6", false)
    val mtu = int("mtu", 9000)
    val bypassLan = bool("bypass_lan", true)
    val perAppMode = str("per_app_mode", PerAppMode.OFF)
    val perAppPackages = strSet("per_app_packages")
    val dnsServers = str("dns_servers", "")
    val testUrl = str("test_url", "http://www.gstatic.com/generate_204")
    val logLevel = str("log_level", "info")
    val themeMode = int("theme_mode", ThemeMode.SYSTEM)
    val language = str("language", "system")
    val userAgent = str("user_agent", "clash.meta mihomo/1.19 Surfmi")
}
