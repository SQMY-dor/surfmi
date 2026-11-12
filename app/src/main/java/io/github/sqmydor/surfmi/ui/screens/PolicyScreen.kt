// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import io.github.sqmydor.surfmi.R
import io.github.sqmydor.surfmi.core.Core
import io.github.sqmydor.surfmi.core.PolicyGroup
import io.github.sqmydor.surfmi.data.Settings
import io.github.sqmydor.surfmi.service.ServiceState
import io.github.sqmydor.surfmi.service.VpnState
import io.github.sqmydor.surfmi.ui.components.delayColor
import io.github.sqmydor.surfmi.ui.components.delayText
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import top.yukonga.miuix.kmp.basic.BasicComponent
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.ScrollBehavior
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.Timer
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.utils.overScrollVertical

@Composable
fun PolicyScreen(padding: PaddingValues, scrollBehavior: ScrollBehavior, testAllSignal: Int) {
    val state by ServiceState.state.collectAsState()
    val mode by Settings.mode.flow.collectAsState()
    var groups by remember { mutableStateOf<List<PolicyGroup>>(emptyList()) }
    val expanded = remember { mutableStateMapOf<String, Boolean>() }
    val testing = remember { mutableStateMapOf<String, Boolean>() }
    val scope = rememberCoroutineScope()

    suspend fun refresh() {
        groups = withContext(Dispatchers.IO) { Core.groups(includeGlobal = mode == "global") }.filterNot { it.hidden }
    }

    fun test(group: PolicyGroup) {
        if (testing[group.name] == true) return
        testing[group.name] = true
        scope.launch {
            withContext(Dispatchers.IO) { Core.testGroup(group.name) }
            refresh()
            testing[group.name] = false
        }
    }

    LaunchedEffect(state, mode) {
        while (state == VpnState.Running) {
            refresh()
            delay(3000)
        }
        groups = emptyList()
    }
    LaunchedEffect(testAllSignal) {
        if (testAllSignal > 0) groups.forEach { test(it) }
    }

    if (state != VpnState.Running) {
        Box(Modifier.fillMaxWidth().padding(padding).padding(32.dp), contentAlignment = Alignment.Center) {
            Text(stringResource(R.string.policy_not_running), color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
        }
        return
    }

    LazyColumn(
        modifier = Modifier.fillMaxHeight().overScrollVertical().nestedScroll(scrollBehavior.nestedScrollConnection),
        contentPadding = padding,
    ) {
        items(groups, key = { it.name }) { g ->
            val isOpen = expanded[g.name] == true
            Card(modifier = Modifier.padding(horizontal = 12.dp).padding(top = 12.dp)) {
                BasicComponent(
                    title = g.name,
                    summary = "${g.type} · ${g.now}",
                    onClick = { expanded[g.name] = !isOpen },
                    endActions = {
                        if (testing[g.name] == true) {
                            InfiniteProgressIndicator()
                        } else {
                            IconButton(onClick = { test(g) }) {
                                Icon(MiuixIcons.Timer, contentDescription = stringResource(R.string.action_test_latency))
                            }
                        }
                    },
                )
                if (isOpen) {
                    Column(Modifier.padding(bottom = 8.dp)) {
                        g.proxies.forEach { p ->
                            val selected = p.name == g.now
                            val onSelect: (() -> Unit)? = if (g.selectable) {
                                {
                                    scope.launch {
                                        withContext(Dispatchers.IO) { Core.select(g.name, p.name) }
                                        refresh()
                                    }
                                    Unit
                                }
                            } else {
                                null
                            }
                            BasicComponent(onClick = onSelect) {
                                Row(
                                    Modifier.fillMaxWidth(),
                                    horizontalArrangement = Arrangement.SpaceBetween,
                                    verticalAlignment = Alignment.CenterVertically,
                                ) {
                                    Column(Modifier.weight(1f)) {
                                        Text(
                                            text = p.name,
                                            fontWeight = if (selected) FontWeight.SemiBold else FontWeight.Normal,
                                            color = if (selected) MiuixTheme.colorScheme.primary else MiuixTheme.colorScheme.onSurface,
                                        )
                                        Text(
                                            text = p.type + if (p.udp) " · UDP" else "",
                                            fontSize = MiuixTheme.textStyles.footnote1.fontSize,
                                            color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                                        )
                                    }
                                    Text(text = delayText(p.delay), color = delayColor(p.delay))
                                }
                            }
                        }
                    }
                }
            }
        }
        item { Box(Modifier.padding(bottom = 12.dp)) }
    }
}
