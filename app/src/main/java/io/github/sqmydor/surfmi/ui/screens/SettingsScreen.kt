// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi.ui.screens

import android.content.Intent
import android.net.Uri
import androidx.appcompat.app.AppCompatDelegate
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.core.os.LocaleListCompat
import io.github.sqmydor.surfmi.BuildConfig
import io.github.sqmydor.surfmi.R
import io.github.sqmydor.surfmi.core.Core
import io.github.sqmydor.surfmi.data.PerAppMode
import io.github.sqmydor.surfmi.data.Pref
import io.github.sqmydor.surfmi.data.Settings
import top.yukonga.miuix.kmp.basic.ButtonDefaults
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.ScrollBehavior
import top.yukonga.miuix.kmp.basic.SmallTitle
import top.yukonga.miuix.kmp.basic.TextButton
import top.yukonga.miuix.kmp.basic.TextField
import top.yukonga.miuix.kmp.overlay.OverlayDialog
import top.yukonga.miuix.kmp.preference.ArrowPreference
import top.yukonga.miuix.kmp.preference.OverlayDropdownPreference
import top.yukonga.miuix.kmp.preference.SwitchPreference
import top.yukonga.miuix.kmp.utils.overScrollVertical

private val STACKS = listOf("gvisor", "system", "mixed")
private val PER_APP = listOf(PerAppMode.OFF, PerAppMode.ALLOW, PerAppMode.BYPASS)
private val LANGS = listOf("system", "en", "zh-CN")
private val LOG_LEVELS = listOf("debug", "info", "warning", "error", "silent")

/** Text-valued setting edited through a dialog. */
private data class TextEdit(val title: String, val pref: Pref<String>)

@Composable
fun SettingsScreen(
    padding: PaddingValues,
    scrollBehavior: ScrollBehavior,
    onOpenAppList: () -> Unit,
    onOpenTemplates: () -> Unit = {},
) {
    val context = LocalContext.current
    val stack by Settings.stack.flow.collectAsState()
    val ipv6 by Settings.ipv6.flow.collectAsState()
    val bypassLan by Settings.bypassLan.flow.collectAsState()
    val perApp by Settings.perAppMode.flow.collectAsState()
    val pkgs by Settings.perAppPackages.flow.collectAsState()
    val dns by Settings.dnsServers.flow.collectAsState()
    val testUrl by Settings.testUrl.flow.collectAsState()
    val ua by Settings.userAgent.flow.collectAsState()
    val theme by Settings.themeMode.flow.collectAsState()
    val lang by Settings.language.flow.collectAsState()
    val logLevel by Settings.logLevel.flow.collectAsState()
    var editing by remember { mutableStateOf<TextEdit?>(null) }
    val restartHint = stringResource(R.string.takes_effect_on_reconnect)

    LazyColumn(
        modifier = Modifier.fillMaxHeight().overScrollVertical().nestedScroll(scrollBehavior.nestedScrollConnection),
        contentPadding = padding,
    ) {
        item { SmallTitle(stringResource(R.string.settings_network)) }
        item {
            Card(Modifier.padding(horizontal = 12.dp)) {
                OverlayDropdownPreference(
                    title = stringResource(R.string.tun_stack),
                    summary = restartHint,
                    items = listOf("gVisor", "System", "Mixed"),
                    selectedIndex = STACKS.indexOf(stack).coerceAtLeast(0),
                    onSelectedIndexChange = { Settings.stack.value = STACKS[it] },
                )
                SwitchPreference(
                    title = stringResource(R.string.ipv6),
                    summary = stringResource(R.string.ipv6_summary),
                    checked = ipv6,
                    onCheckedChange = { Settings.ipv6.value = it },
                )
                SwitchPreference(
                    title = stringResource(R.string.bypass_lan),
                    summary = stringResource(R.string.bypass_lan_summary),
                    checked = bypassLan,
                    onCheckedChange = { Settings.bypassLan.value = it },
                )
                ArrowPreference(
                    title = stringResource(R.string.dns_servers),
                    summary = dns.ifBlank { stringResource(R.string.dns_from_profile) },
                    onClick = { editing = TextEdit(context.getString(R.string.dns_servers), Settings.dnsServers) },
                )
                ArrowPreference(
                    title = stringResource(R.string.test_url),
                    summary = testUrl,
                    onClick = { editing = TextEdit(context.getString(R.string.test_url), Settings.testUrl) },
                )
            }
        }

        item { SmallTitle(stringResource(R.string.per_app_proxy)) }
        item {
            Card(Modifier.padding(horizontal = 12.dp)) {
                OverlayDropdownPreference(
                    title = stringResource(R.string.per_app_mode),
                    items = listOf(
                        stringResource(R.string.per_app_off),
                        stringResource(R.string.per_app_allow),
                        stringResource(R.string.per_app_bypass),
                    ),
                    selectedIndex = PER_APP.indexOf(perApp).coerceAtLeast(0),
                    onSelectedIndexChange = { Settings.perAppMode.value = PER_APP[it] },
                )
                ArrowPreference(
                    title = stringResource(R.string.per_app_select),
                    summary = stringResource(R.string.per_app_selected_count, pkgs.size),
                    enabled = perApp != PerAppMode.OFF,
                    onClick = onOpenAppList,
                )
            }
        }

        item { SmallTitle(stringResource(R.string.settings_subscription)) }
        item {
            Card(Modifier.padding(horizontal = 12.dp)) {
                ArrowPreference(
                    title = stringResource(R.string.user_agent),
                    summary = ua,
                    onClick = { editing = TextEdit(context.getString(R.string.user_agent), Settings.userAgent) },
                )
                ArrowPreference(
                    title = stringResource(R.string.rule_templates),
                    summary = stringResource(R.string.rule_templates_summary),
                    onClick = onOpenTemplates,
                )
            }
        }

        item { SmallTitle(stringResource(R.string.settings_appearance)) }
        item {
            Card(Modifier.padding(horizontal = 12.dp)) {
                OverlayDropdownPreference(
                    title = stringResource(R.string.theme),
                    items = listOf(
                        stringResource(R.string.theme_system),
                        stringResource(R.string.theme_light),
                        stringResource(R.string.theme_dark),
                        stringResource(R.string.theme_monet),
                    ),
                    selectedIndex = theme,
                    onSelectedIndexChange = { Settings.themeMode.value = it },
                )
                OverlayDropdownPreference(
                    title = stringResource(R.string.language),
                    items = listOf(stringResource(R.string.language_system), "English", "简体中文"),
                    selectedIndex = LANGS.indexOf(lang).coerceAtLeast(0),
                    onSelectedIndexChange = {
                        Settings.language.value = LANGS[it]
                        AppCompatDelegate.setApplicationLocales(
                            if (LANGS[it] == "system") LocaleListCompat.getEmptyLocaleList() else LocaleListCompat.forLanguageTags(LANGS[it]),
                        )
                    },
                )
            }
        }

        item { SmallTitle(stringResource(R.string.settings_advanced)) }
        item {
            Card(Modifier.padding(horizontal = 12.dp)) {
                OverlayDropdownPreference(
                    title = stringResource(R.string.log_level),
                    items = LOG_LEVELS,
                    selectedIndex = LOG_LEVELS.indexOf(logLevel).coerceAtLeast(0),
                    onSelectedIndexChange = { Settings.logLevel.value = LOG_LEVELS[it] },
                )
            }
        }

        item { SmallTitle(stringResource(R.string.about)) }
        item {
            Card(Modifier.padding(horizontal = 12.dp).padding(bottom = 12.dp)) {
                ArrowPreference(
                    title = "${BuildConfig.APP_NAME} ${BuildConfig.VERSION_NAME}",
                    summary = Core.version(),
                    onClick = {
                        context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse("https://github.com/SQMY-dor/surfmi")))
                    },
                )
                ArrowPreference(
                    title = stringResource(R.string.license),
                    summary = stringResource(R.string.license_summary),
                )
            }
        }
    }

    val e = editing
    var draft by remember(e) { mutableStateOf(e?.pref?.value ?: "") }
    OverlayDialog(show = e != null, title = e?.title, onDismissRequest = { editing = null }) {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            TextField(value = draft, onValueChange = { draft = it }, singleLine = true)
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                TextButton(text = stringResource(android.R.string.cancel), onClick = { editing = null }, modifier = Modifier.weight(1f))
                TextButton(
                    text = stringResource(android.R.string.ok),
                    colors = ButtonDefaults.textButtonColorsPrimary(),
                    modifier = Modifier.weight(1f),
                    onClick = {
                        e?.pref?.value = draft.trim()
                        editing = null
                    },
                )
            }
        }
    }
}
