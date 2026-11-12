// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi.ui.screens

import android.net.Uri
import android.provider.OpenableColumns
import android.widget.Toast
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import io.github.sqmydor.surfmi.R
import io.github.sqmydor.surfmi.core.libcore.Libcore
import io.github.sqmydor.surfmi.data.Profile
import io.github.sqmydor.surfmi.data.ProfileStore
import io.github.sqmydor.surfmi.data.Settings
import io.github.sqmydor.surfmi.util.formatBytes
import io.github.sqmydor.surfmi.util.formatDate
import io.github.sqmydor.surfmi.util.formatDay
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import top.yukonga.miuix.kmp.basic.ButtonDefaults
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.CardDefaults
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.LinearProgressIndicator
import top.yukonga.miuix.kmp.basic.ScrollBehavior
import top.yukonga.miuix.kmp.basic.TabRowWithContour
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TextButton
import top.yukonga.miuix.kmp.basic.TextField
import top.yukonga.miuix.kmp.overlay.OverlayDialog
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.utils.overScrollVertical

@Composable
fun ProfilesScreen(
    padding: PaddingValues,
    scrollBehavior: ScrollBehavior,
    showAdd: Boolean,
    initialUrl: String?,
    onAddDismiss: () -> Unit,
    onEdit: (String) -> Unit,
) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val profiles by ProfileStore.profiles.collectAsState()
    val activeId by Settings.activeProfile.flow.collectAsState()
    var busy by remember { mutableStateOf<String?>(null) }
    var actionsFor by remember { mutableStateOf<Profile?>(null) }

    fun toast(msg: String) = Toast.makeText(context, msg, Toast.LENGTH_LONG).show()

    fun runUpdate(p: Profile) {
        busy = p.id
        scope.launch {
            runCatching { ProfileStore.update(p) }
                .onSuccess { toast(context.getString(R.string.profile_updated, it.name, it.warnings.size)) }
                .onFailure { toast(context.getString(R.string.profile_update_failed, it.message ?: "")) }
            busy = null
        }
    }

    fun activate(p: Profile) {
        ProfileStore.setActive(p)
        if (Libcore.isRunning()) {
            scope.launch(Dispatchers.IO) {
                runCatching { Libcore.reload(ProfileStore.configFile(p).absolutePath) }
                    .onFailure { withContext(Dispatchers.Main) { toast(it.message ?: "reload failed") } }
            }
        }
    }

    if (profiles.isEmpty()) {
        Box(Modifier.fillMaxWidth().padding(padding).padding(32.dp), contentAlignment = Alignment.Center) {
            Text(stringResource(R.string.no_profile_hint), color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
        }
    } else {
        LazyColumn(
            modifier = Modifier.fillMaxHeight().overScrollVertical().nestedScroll(scrollBehavior.nestedScrollConnection),
            contentPadding = padding,
        ) {
            items(profiles, key = { it.id }) { p ->
                ProfileCard(
                    profile = p,
                    active = p.id == activeId || (activeId.isEmpty() && p == profiles.first()),
                    updating = busy == p.id,
                    onClick = { activate(p) },
                    onLongPress = { actionsFor = p },
                )
            }
            item { Spacer(Modifier.padding(bottom = 12.dp)) }
        }
    }

    // ------------------------------------------------ profile actions
    val target = actionsFor
    OverlayDialog(
        show = target != null,
        title = target?.name,
        onDismissRequest = { actionsFor = null },
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            if (target?.isRemote == true) {
                TextButton(
                    text = stringResource(R.string.action_update),
                    onClick = { actionsFor = null; runUpdate(target) },
                    modifier = Modifier.fillMaxWidth(),
                )
            }
            TextButton(
                text = stringResource(R.string.action_edit),
                onClick = { actionsFor = null; target?.let { onEdit(it.id) } },
                modifier = Modifier.fillMaxWidth(),
            )
            TextButton(
                text = stringResource(R.string.action_delete),
                onClick = { actionsFor = null; target?.let { ProfileStore.delete(it) } },
                modifier = Modifier.fillMaxWidth(),
            )
        }
    }

    AddProfileDialog(
        show = showAdd,
        initialUrl = initialUrl,
        onDismiss = onAddDismiss,
        onAdded = { toast(context.getString(R.string.profile_added, it.name, it.proxies, it.warnings.size)) },
        onError = { toast(it) },
    )
}

@Composable
private fun ProfileCard(profile: Profile, active: Boolean, updating: Boolean, onClick: () -> Unit, onLongPress: () -> Unit) {
    Card(
        modifier = Modifier.padding(horizontal = 12.dp).padding(top = 12.dp),
        onClick = onClick,
        onLongPress = onLongPress,
        insideMargin = PaddingValues(16.dp),
        colors = if (active) CardDefaults.defaultColors(color = MiuixTheme.colorScheme.primaryContainer) else CardDefaults.defaultColors(),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                Text(profile.name, fontWeight = FontWeight.SemiBold)
                Text(
                    text = listOfNotNull(
                        profile.format.ifBlank { null },
                        stringResource(R.string.profile_counts, profile.proxies, profile.groups, profile.rules),
                    ).joinToString(" · "),
                    fontSize = MiuixTheme.textStyles.footnote1.fontSize,
                    color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                )
            }
            if (updating) InfiniteProgressIndicator()
        }
        if (profile.total > 0) {
            val used = profile.upload + profile.download
            LinearProgressIndicator(
                progress = (used.toDouble() / profile.total).toFloat().coerceIn(0f, 1f),
                modifier = Modifier.fillMaxWidth().padding(vertical = 8.dp),
            )
            Text(
                text = stringResource(R.string.usage_summary, formatBytes(used), formatBytes(profile.total), formatDay(profile.expire)),
                fontSize = MiuixTheme.textStyles.footnote1.fontSize,
                color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
            )
        }
        Text(
            text = stringResource(R.string.last_updated, formatDate(profile.lastUpdated)) +
                if (profile.warnings.isNotEmpty()) " · " + stringResource(R.string.warning_count, profile.warnings.size) else "",
            fontSize = MiuixTheme.textStyles.footnote2.fontSize,
            color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
            modifier = Modifier.padding(top = 4.dp),
        )
    }
}

@Composable
private fun AddProfileDialog(
    show: Boolean,
    initialUrl: String?,
    onDismiss: () -> Unit,
    onAdded: (Profile) -> Unit,
    onError: (String) -> Unit,
) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var kind by remember { mutableIntStateOf(0) } // 0 URL, 1 file, 2 paste
    var name by remember { mutableStateOf("") }
    var url by remember { mutableStateOf("") }
    var hours by remember { mutableStateOf("24") }
    var pasted by remember { mutableStateOf("") }
    var working by remember { mutableStateOf(false) }

    LaunchedEffect(show, initialUrl) {
        if (show && initialUrl != null) {
            kind = 0
            url = initialUrl
        }
    }

    fun finish(block: suspend () -> Profile) {
        working = true
        scope.launch {
            runCatching { block() }
                .onSuccess {
                    onAdded(it)
                    name = ""; url = ""; pasted = ""
                    onDismiss()
                }
                .onFailure { onError(context.getString(R.string.profile_add_failed, it.message ?: "")) }
            working = false
        }
    }

    val picker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri: Uri? ->
        uri ?: return@rememberLauncherForActivityResult
        finish {
            val bytes = withContext(Dispatchers.IO) {
                context.contentResolver.openInputStream(uri)?.use { it.readBytes() } ?: error("cannot read file")
            }
            ProfileStore.addLocal(name.ifBlank { displayName(context, uri) ?: "Imported" }, bytes)
        }
    }

    OverlayDialog(show = show, title = stringResource(R.string.action_add_profile), onDismissRequest = { if (!working) onDismiss() }) {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            TabRowWithContour(
                tabs = listOf(stringResource(R.string.add_by_url), stringResource(R.string.add_by_file), stringResource(R.string.add_by_paste)),
                selectedTabIndex = kind,
                onTabSelected = { kind = it },
            )
            TextField(value = name, onValueChange = { name = it }, label = stringResource(R.string.profile_name), singleLine = true)
            when (kind) {
                0 -> {
                    TextField(
                        value = url,
                        onValueChange = { url = it },
                        label = stringResource(R.string.subscription_url),
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
                    )
                    TextField(
                        value = hours,
                        onValueChange = { v -> hours = v.filter { it.isDigit() } },
                        label = stringResource(R.string.auto_update_hours),
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    )
                }
                1 -> Text(stringResource(R.string.add_by_file_hint), color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
                else -> TextField(
                    value = pasted,
                    onValueChange = { pasted = it },
                    label = stringResource(R.string.add_by_paste_hint),
                    modifier = Modifier.heightIn(min = 120.dp),
                )
            }
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically) {
                TextButton(
                    text = stringResource(android.R.string.cancel),
                    onClick = onDismiss,
                    enabled = !working,
                    modifier = Modifier.weight(1f),
                )
                if (working) {
                    Box(Modifier.width(48.dp), contentAlignment = Alignment.Center) { InfiniteProgressIndicator() }
                }
                TextButton(
                    text = stringResource(if (kind == 1) R.string.choose_file else R.string.action_add),
                    enabled = !working,
                    colors = ButtonDefaults.textButtonColorsPrimary(),
                    modifier = Modifier.weight(1f),
                    onClick = {
                        when (kind) {
                            0 -> if (url.isBlank()) {
                                onError(context.getString(R.string.error_url_empty))
                            } else {
                                finish { ProfileStore.addRemote(name, url, hours.toIntOrNull() ?: 0, "") }
                            }
                            1 -> picker.launch(arrayOf("*/*"))
                            else -> if (pasted.isBlank()) {
                                onError(context.getString(R.string.error_content_empty))
                            } else {
                                finish { ProfileStore.addLocal(name.ifBlank { "Pasted" }, pasted.toByteArray()) }
                            }
                        }
                    },
                )
            }
        }
    }
}

private fun displayName(context: android.content.Context, uri: Uri): String? = runCatching {
    context.contentResolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)?.use { c ->
        if (c.moveToFirst()) c.getString(0)?.substringBeforeLast('.') else null
    }
}.getOrNull()
