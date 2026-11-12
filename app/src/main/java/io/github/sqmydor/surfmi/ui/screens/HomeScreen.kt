// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import io.github.sqmydor.surfmi.R
import io.github.sqmydor.surfmi.core.Core
import io.github.sqmydor.surfmi.data.ProfileStore
import io.github.sqmydor.surfmi.data.Settings
import io.github.sqmydor.surfmi.service.ServiceState
import io.github.sqmydor.surfmi.service.VpnState
import io.github.sqmydor.surfmi.ui.components.StatBlock
import io.github.sqmydor.surfmi.util.formatBytes
import io.github.sqmydor.surfmi.util.formatDay
import io.github.sqmydor.surfmi.util.formatDuration
import io.github.sqmydor.surfmi.util.formatSpeed
import kotlinx.coroutines.delay
import top.yukonga.miuix.kmp.basic.BasicComponent
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.ScrollBehavior
import top.yukonga.miuix.kmp.basic.SmallTitle
import top.yukonga.miuix.kmp.basic.Switch
import top.yukonga.miuix.kmp.basic.TabRowWithContour
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.preference.ArrowPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.utils.overScrollVertical

private val MODES = listOf("rule", "global", "direct")

@Composable
fun HomeScreen(
    padding: PaddingValues,
    scrollBehavior: ScrollBehavior,
    onConnect: () -> Unit,
    onDisconnect: () -> Unit,
    onOpenProfiles: () -> Unit,
) {
    val state by ServiceState.state.collectAsState()
    val error by ServiceState.error.collectAsState()
    val traffic by ServiceState.traffic.collectAsState()
    val startedAt by ServiceState.startedAt.collectAsState()
    val profiles by ProfileStore.profiles.collectAsState()
    val activeId by Settings.activeProfile.flow.collectAsState()
    val mode by Settings.mode.flow.collectAsState()
    val active = profiles.firstOrNull { it.id == activeId } ?: profiles.firstOrNull()

    var now by remember { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(state) {
        while (state == VpnState.Running) {
            now = System.currentTimeMillis()
            delay(1000)
        }
    }

    val running = state == VpnState.Running
    val busy = state == VpnState.Starting || state == VpnState.Stopping

    LazyColumn(
        modifier = Modifier.fillMaxHeight().overScrollVertical().nestedScroll(scrollBehavior.nestedScrollConnection),
        contentPadding = padding,
    ) {
        item {
            Card(modifier = Modifier.padding(horizontal = 12.dp).padding(top = 12.dp)) {
                BasicComponent(
                    title = stringResource(
                        when (state) {
                            VpnState.Running -> R.string.status_connected
                            VpnState.Starting -> R.string.status_connecting
                            VpnState.Stopping -> R.string.status_disconnecting
                            VpnState.Stopped -> R.string.status_disconnected
                        },
                    ),
                    summary = if (running) formatDuration(now - startedAt) else active?.name ?: stringResource(R.string.no_profile),
                    endActions = {
                        Switch(
                            checked = running || state == VpnState.Starting,
                            enabled = !busy,
                            onCheckedChange = { on -> if (on) onConnect() else onDisconnect() },
                        )
                    },
                )
                error?.let {
                    Text(
                        text = it,
                        color = Color(0xFFE5484D),
                        modifier = Modifier.padding(start = 16.dp, end = 16.dp, bottom = 12.dp),
                    )
                }
            }
        }

        item { SmallTitle(text = stringResource(R.string.outbound_mode)) }
        item {
            Card(modifier = Modifier.padding(horizontal = 12.dp)) {
                TabRowWithContour(
                    tabs = listOf(stringResource(R.string.mode_rule), stringResource(R.string.mode_global), stringResource(R.string.mode_direct)),
                    selectedTabIndex = MODES.indexOf(mode).coerceAtLeast(0),
                    onTabSelected = { i ->
                        Settings.mode.value = MODES[i]
                        Core.setMode(MODES[i])
                    },
                    modifier = Modifier.padding(12.dp),
                )
            }
        }

        item { SmallTitle(text = stringResource(R.string.traffic)) }
        item {
            Card(modifier = Modifier.padding(horizontal = 12.dp)) {
                Row(Modifier.fillMaxWidth().padding(16.dp), horizontalArrangement = Arrangement.SpaceBetween) {
                    StatBlock(stringResource(R.string.upload), formatSpeed(traffic.up), Modifier.weight(1f))
                    StatBlock(stringResource(R.string.download), formatSpeed(traffic.down), Modifier.weight(1f))
                }
                Row(Modifier.fillMaxWidth().padding(start = 16.dp, end = 16.dp, bottom = 16.dp), horizontalArrangement = Arrangement.SpaceBetween) {
                    StatBlock(stringResource(R.string.upload_total), formatBytes(traffic.upTotal), Modifier.weight(1f))
                    StatBlock(stringResource(R.string.download_total), formatBytes(traffic.downTotal), Modifier.weight(1f))
                }
            }
        }

        item { SmallTitle(text = stringResource(R.string.current_profile)) }
        item {
            Card(modifier = Modifier.padding(horizontal = 12.dp).padding(bottom = 12.dp)) {
                val summary = when {
                    active == null -> stringResource(R.string.no_profile_hint)
                    active.total > 0 -> stringResource(
                        R.string.usage_summary,
                        formatBytes(active.upload + active.download),
                        formatBytes(active.total),
                        formatDay(active.expire),
                    )
                    else -> stringResource(R.string.profile_counts, active.proxies, active.groups, active.rules)
                }
                ArrowPreference(
                    title = active?.name ?: stringResource(R.string.no_profile),
                    summary = summary,
                    onClick = onOpenProfiles,
                )
            }
        }

        item {
            Text(
                text = Core.version(),
                fontSize = MiuixTheme.textStyles.footnote2.fontSize,
                color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                modifier = Modifier.padding(horizontal = 24.dp, vertical = 8.dp),
            )
        }
    }
}
