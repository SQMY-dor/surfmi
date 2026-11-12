// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi.ui.screens

import android.widget.Toast
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
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
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.sqmydor.surfmi.R
import io.github.sqmydor.surfmi.data.ProfileStore
import io.github.sqmydor.surfmi.data.TemplateMode
import io.github.sqmydor.surfmi.data.TemplateStore
import kotlinx.coroutines.launch
import top.yukonga.miuix.kmp.basic.ButtonDefaults
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.SmallTitle
import top.yukonga.miuix.kmp.basic.TabRowWithContour
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TextButton
import top.yukonga.miuix.kmp.basic.TextField
import top.yukonga.miuix.kmp.basic.TopAppBar
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.Back
import top.yukonga.miuix.kmp.preference.ArrowPreference
import top.yukonga.miuix.kmp.preference.OverlayDropdownPreference
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.utils.overScrollVertical

@Composable
fun ProfileEditScreen(profileId: String, onBack: () -> Unit, onOpenTemplates: () -> Unit = {}) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val profile = remember(profileId) { ProfileStore.get(profileId) }
    if (profile == null) {
        LaunchedEffect(Unit) { onBack() }
        return
    }
    var tab by remember { mutableIntStateOf(0) }
    var name by remember { mutableStateOf(profile.name) }
    var url by remember { mutableStateOf(profile.url) }
    var hours by remember { mutableStateOf(profile.updateHours.toString()) }
    var ua by remember { mutableStateOf(profile.userAgent) }
    var source by remember { mutableStateOf(ProfileStore.readSource(profile)) }
    var config by remember { mutableStateOf(ProfileStore.readConfig(profile)) }
    var warnings by remember { mutableStateOf(profile.warnings) }
    var saving by remember { mutableStateOf(false) }
    val templates by TemplateStore.templates.collectAsState()
    var templateId by remember { mutableStateOf(profile.templateId) }
    var templateMode by remember { mutableStateOf(profile.templateMode) }
    var applying by remember { mutableStateOf(false) }

    fun applyTemplate(id: String, mode: String) {
        val prevId = templateId
        val prevMode = templateMode
        templateId = id
        templateMode = mode
        applying = true
        scope.launch {
            val current = ProfileStore.get(profile.id) ?: profile
            runCatching { ProfileStore.setTemplate(current, id, mode) }
                .onSuccess {
                    config = ProfileStore.readConfig(it)
                    warnings = it.warnings
                    Toast.makeText(context, context.getString(R.string.template_applied, it.groups, it.rules), Toast.LENGTH_SHORT).show()
                }
                .onFailure {
                    templateId = prevId
                    templateMode = prevMode
                    Toast.makeText(context, context.getString(R.string.template_apply_failed, it.message ?: ""), Toast.LENGTH_LONG).show()
                }
            applying = false
        }
    }
    val scroll = MiuixScrollBehavior()
    val mono = TextStyle(fontFamily = FontFamily.Monospace, fontSize = 12.sp, color = MiuixTheme.colorScheme.onSurface)

    Scaffold(
        topBar = {
            TopAppBar(
                title = profile.name,
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
                TabRowWithContour(
                    tabs = listOf(stringResource(R.string.edit_info), stringResource(R.string.edit_source), stringResource(R.string.edit_generated)),
                    selectedTabIndex = tab,
                    onTabSelected = { tab = it },
                    modifier = Modifier.padding(12.dp),
                )
            }
            when (tab) {
                0 -> item {
                    Card(modifier = Modifier.padding(horizontal = 12.dp), insideMargin = androidx.compose.foundation.layout.PaddingValues(16.dp)) {
                        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                            TextField(value = name, onValueChange = { name = it }, label = stringResource(R.string.profile_name), singleLine = true)
                            if (profile.isRemote) {
                                TextField(value = url, onValueChange = { url = it }, label = stringResource(R.string.subscription_url))
                                TextField(
                                    value = hours,
                                    onValueChange = { v -> hours = v.filter { it.isDigit() } },
                                    label = stringResource(R.string.auto_update_hours),
                                    singleLine = true,
                                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                                )
                                TextField(value = ua, onValueChange = { ua = it }, label = stringResource(R.string.user_agent_override), singleLine = true)
                            }
                            TextButton(
                                text = stringResource(R.string.action_save),
                                colors = ButtonDefaults.textButtonColorsPrimary(),
                                modifier = Modifier.fillMaxWidth(),
                                onClick = {
                                    ProfileStore.rename(profile, name.ifBlank { profile.name }, url.trim(), hours.toIntOrNull() ?: 0, ua.trim())
                                    Toast.makeText(context, R.string.saved, Toast.LENGTH_SHORT).show()
                                },
                            )
                        }
                    }
                    SmallTitle(text = stringResource(R.string.template_section))
                    Card(modifier = Modifier.padding(horizontal = 12.dp)) {
                        val noneLabel = stringResource(R.string.template_none)
                        val missing = templateId.isNotBlank() && templates.none { it.id == templateId }
                        val labels = listOf(noneLabel) + templates.map { it.name } +
                            if (missing) listOf(stringResource(R.string.template_missing)) else emptyList()
                        val selected = when {
                            templateId.isBlank() -> 0
                            missing -> labels.size - 1
                            else -> 1 + templates.indexOfFirst { it.id == templateId }
                        }
                        OverlayDropdownPreference(
                            title = stringResource(R.string.template_replace),
                            summary = stringResource(if (applying) R.string.template_applying else R.string.template_replace_summary),
                            items = labels,
                            selectedIndex = selected,
                            enabled = !applying,
                            onSelectedIndexChange = { i ->
                                val id = if (i == 0 || i > templates.size) "" else templates[i - 1].id
                                if (id != templateId) applyTemplate(id, templateMode)
                            },
                        )
                        OverlayDropdownPreference(
                            title = stringResource(R.string.template_mode),
                            items = listOf(
                                stringResource(R.string.template_mode_replace),
                                stringResource(R.string.template_mode_append),
                                stringResource(R.string.template_mode_prepend),
                            ),
                            selectedIndex = TemplateMode.ALL.indexOf(templateMode).coerceAtLeast(0),
                            enabled = !applying && templateId.isNotBlank(),
                            onSelectedIndexChange = { i ->
                                val mode = TemplateMode.ALL[i]
                                if (mode != templateMode) applyTemplate(templateId, mode)
                            },
                        )
                        ArrowPreference(
                            title = stringResource(R.string.rule_templates),
                            summary = stringResource(R.string.rule_templates_manage),
                            onClick = onOpenTemplates,
                        )
                    }
                    if (warnings.isNotEmpty()) {
                        SmallTitle(text = stringResource(R.string.conversion_warnings))
                        Card(modifier = Modifier.padding(horizontal = 12.dp).padding(bottom = 12.dp), insideMargin = androidx.compose.foundation.layout.PaddingValues(16.dp)) {
                            warnings.forEach { w ->
                                Text("• $w", fontSize = MiuixTheme.textStyles.footnote1.fontSize, modifier = Modifier.padding(vertical = 2.dp))
                            }
                        }
                    }
                }
                1 -> item {
                    Column(Modifier.padding(horizontal = 12.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                        TextField(
                            value = source,
                            onValueChange = { source = it },
                            textStyle = mono,
                            modifier = Modifier.fillMaxWidth().heightIn(min = 320.dp),
                        )
                        TextButton(
                            text = stringResource(if (saving) R.string.saving else R.string.action_save_convert),
                            enabled = !saving,
                            colors = ButtonDefaults.textButtonColorsPrimary(),
                            modifier = Modifier.fillMaxWidth().padding(bottom = 12.dp),
                            onClick = {
                                saving = true
                                scope.launch {
                                    runCatching { ProfileStore.saveSource(profile, source) }
                                        .onSuccess {
                                            config = ProfileStore.readConfig(it)
                                            warnings = it.warnings
                                            Toast.makeText(context, R.string.saved, Toast.LENGTH_SHORT).show()
                                        }
                                        .onFailure {
                                            Toast.makeText(context, context.getString(R.string.profile_add_failed, it.message ?: ""), Toast.LENGTH_LONG).show()
                                        }
                                    saving = false
                                }
                            },
                        )
                    }
                }
                else -> item {
                    Card(modifier = Modifier.padding(horizontal = 12.dp).padding(bottom = 12.dp), insideMargin = androidx.compose.foundation.layout.PaddingValues(12.dp)) {
                        Text(text = config, style = mono)
                    }
                }
            }
        }
    }
}
