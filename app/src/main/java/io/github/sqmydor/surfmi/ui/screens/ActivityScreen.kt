// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi.ui.screens

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import io.github.sqmydor.surfmi.R
import io.github.sqmydor.surfmi.core.ConnectionInfo
import io.github.sqmydor.surfmi.core.Core
import io.github.sqmydor.surfmi.service.LogBuffer
import io.github.sqmydor.surfmi.service.LogLine
import io.github.sqmydor.surfmi.service.ServiceState
import io.github.sqmydor.surfmi.service.VpnState
import io.github.sqmydor.surfmi.util.formatBytes
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.ScrollBehavior
import top.yukonga.miuix.kmp.basic.TabRowWithContour
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.utils.overScrollVertical
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

private val timeFmt = SimpleDateFormat("HH:mm:ss", Locale.ROOT)

@Composable
fun ActivityScreen(padding: PaddingValues, scrollBehavior: ScrollBehavior, clearSignal: Int) {
    var tab by rememberSaveable { mutableIntStateOf(0) }
    val state by ServiceState.state.collectAsState()
    val logVersion by LogBuffer.version.collectAsState()
    var conns by remember { mutableStateOf<List<ConnectionInfo>>(emptyList()) }
    val logs = remember(logVersion) { LogBuffer.snapshot().asReversed() }

    LaunchedEffect(state, tab) {
        while (state == VpnState.Running && tab == 0) {
            conns = withContext(Dispatchers.IO) { Core.connections() }
            delay(1000)
        }
    }
    LaunchedEffect(clearSignal) {
        if (clearSignal == 0) return@LaunchedEffect
        if (tab == 0) {
            withContext(Dispatchers.IO) { Core.closeAll() }
            conns = emptyList()
        } else {
            LogBuffer.clear()
        }
    }

    LazyColumn(
        modifier = Modifier.fillMaxHeight().overScrollVertical().nestedScroll(scrollBehavior.nestedScrollConnection),
        contentPadding = padding,
    ) {
        item {
            TabRowWithContour(
                tabs = listOf(stringResource(R.string.tab_requests), stringResource(R.string.tab_logs)),
                selectedTabIndex = tab,
                onTabSelected = { tab = it },
                modifier = Modifier.padding(12.dp),
            )
        }
        if (tab == 0) {
            if (conns.isEmpty()) {
                item { EmptyHint(stringResource(R.string.no_requests)) }
            }
            items(conns, key = { it.id }) { c -> ConnectionRow(c) }
        } else {
            if (logs.isEmpty()) {
                item { EmptyHint(stringResource(R.string.no_logs)) }
            }
            items(logs.take(500)) { l -> LogRow(l) }
        }
    }
}

@Composable
private fun EmptyHint(text: String) {
    Text(text, color = MiuixTheme.colorScheme.onSurfaceVariantSummary, modifier = Modifier.padding(24.dp))
}

@Composable
private fun ConnectionRow(c: ConnectionInfo) {
    Card(
        modifier = Modifier.padding(horizontal = 12.dp).padding(bottom = 8.dp),
        insideMargin = PaddingValues(12.dp),
        onLongPress = { Core.closeConnection(c.id) },
    ) {
        Text(c.host, maxLines = 1)
        val rule = if (c.rulePayload.isNotBlank()) "${c.rule}(${c.rulePayload})" else c.rule
        Text(
            text = listOf(c.network.uppercase(), c.process.ifBlank { "-" }, rule, c.chains.reversed().joinToString(" → ")).joinToString(" · "),
            fontSize = MiuixTheme.textStyles.footnote1.fontSize,
            color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
        )
        Text(
            text = "↑ ${formatBytes(c.upload)}  ↓ ${formatBytes(c.download)}  ${timeFmt.format(Date(c.start))}",
            fontSize = MiuixTheme.textStyles.footnote2.fontSize,
            color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
        )
    }
}

@Composable
private fun LogRow(l: LogLine) {
    val color = when (l.level) {
        "error" -> Color(0xFFE5484D)
        "warning" -> Color(0xFFE69A28)
        "debug" -> Color(0xFF9E9E9E)
        else -> MiuixTheme.colorScheme.onSurface
    }
    Column(Modifier.padding(horizontal = 16.dp, vertical = 4.dp)) {
        Text(
            text = "${timeFmt.format(Date(l.time))}  ${l.level.uppercase()}",
            fontSize = MiuixTheme.textStyles.footnote2.fontSize,
            color = color,
        )
        Text(text = l.message, fontFamily = FontFamily.Monospace, fontSize = MiuixTheme.textStyles.footnote1.fontSize)
    }
}
