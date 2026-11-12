// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi.ui.screens

import android.content.pm.ApplicationInfo
import android.content.pm.PackageManager
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
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
import io.github.sqmydor.surfmi.R
import io.github.sqmydor.surfmi.data.Settings
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.TextField
import top.yukonga.miuix.kmp.basic.TopAppBar
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.Back
import top.yukonga.miuix.kmp.preference.CheckboxPreference
import top.yukonga.miuix.kmp.preference.SwitchPreference
import top.yukonga.miuix.kmp.utils.overScrollVertical

private data class AppEntry(val label: String, val pkg: String, val system: Boolean)

/** Choose the apps used by the per-app allow / bypass list. */
@Composable
fun AppListScreen(onBack: () -> Unit) {
    val context = LocalContext.current
    val selected by Settings.perAppPackages.flow.collectAsState()
    var apps by remember { mutableStateOf<List<AppEntry>?>(null) }
    var query by remember { mutableStateOf("") }
    var showSystem by remember { mutableStateOf(false) }
    val scroll = MiuixScrollBehavior()

    LaunchedEffect(Unit) {
        apps = withContext(Dispatchers.IO) {
            val pm = context.packageManager
            pm.getInstalledApplications(PackageManager.GET_META_DATA)
                .filter { it.packageName != context.packageName }
                .map { AppEntry(it.loadLabel(pm).toString(), it.packageName, it.flags and ApplicationInfo.FLAG_SYSTEM != 0) }
                .sortedBy { it.label.lowercase() }
        }
    }

    val visible = apps.orEmpty()
        .filter { showSystem || !it.system || it.pkg in selected }
        .filter { query.isBlank() || it.label.contains(query, true) || it.pkg.contains(query, true) }
        .sortedByDescending { it.pkg in selected }

    Scaffold(
        topBar = {
            TopAppBar(
                title = stringResource(R.string.per_app_select),
                scrollBehavior = scroll,
                navigationIcon = {
                    IconButton(onClick = onBack) { Icon(MiuixIcons.Back, contentDescription = stringResource(R.string.action_back)) }
                },
            )
        },
    ) { padding ->
        LazyColumn(
            modifier = Modifier.fillMaxHeight().overScrollVertical().nestedScroll(scroll.nestedScrollConnection),
            contentPadding = padding,
        ) {
            item {
                TextField(
                    value = query,
                    onValueChange = { query = it },
                    label = stringResource(R.string.search_apps),
                    singleLine = true,
                    modifier = Modifier.padding(12.dp),
                )
            }
            item {
                SwitchPreference(
                    title = stringResource(R.string.show_system_apps),
                    checked = showSystem,
                    onCheckedChange = { showSystem = it },
                )
            }
            if (apps == null) {
                item { InfiniteProgressIndicator(modifier = Modifier.padding(24.dp)) }
            }
            items(visible, key = { it.pkg }) { app ->
                CheckboxPreference(
                    title = app.label,
                    summary = app.pkg,
                    checked = app.pkg in selected,
                    onCheckedChange = { on ->
                        Settings.perAppPackages.value = if (on) selected + app.pkg else selected - app.pkg
                    },
                )
            }
        }
    }
}
