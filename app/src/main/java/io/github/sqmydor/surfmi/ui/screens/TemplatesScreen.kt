// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi.ui.screens

import android.net.Uri
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
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.sqmydor.surfmi.R
import io.github.sqmydor.surfmi.data.ProfileStore
import io.github.sqmydor.surfmi.data.RuleTemplate
import io.github.sqmydor.surfmi.data.TemplateStore
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import top.yukonga.miuix.kmp.basic.ButtonDefaults
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.SmallTitle
import top.yukonga.miuix.kmp.basic.TabRowWithContour
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TextButton
import top.yukonga.miuix.kmp.basic.TextField
import top.yukonga.miuix.kmp.basic.TopAppBar
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.Add
import top.yukonga.miuix.kmp.icon.extended.Back
import top.yukonga.miuix.kmp.overlay.OverlayDialog
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.utils.overScrollVertical

/** "规则模板": lists built-in and user rule templates. */
@Composable
fun TemplatesScreen(onBack: () -> Unit, onEdit: (String?) -> Unit) {
    val context = LocalContext.current
    val templates by TemplateStore.templates.collectAsState()
    val profiles by ProfileStore.profiles.collectAsState()
    var showAdd by remember { mutableStateOf(false) }
    var actionsFor by remember { mutableStateOf<RuleTemplate?>(null) }
    val scroll = MiuixScrollBehavior()

    Scaffold(
        topBar = {
            TopAppBar(
                title = stringResource(R.string.rule_templates),
                scrollBehavior = scroll,
                navigationIcon = {
                    IconButton(onClick = onBack) { Icon(MiuixIcons.Back, contentDescription = stringResource(R.string.action_back)) }
                },
                actions = {
                    IconButton(onClick = { showAdd = true }) {
                        Icon(MiuixIcons.Add, contentDescription = stringResource(R.string.template_add))
                    }
                },
            )
        },
    ) { padding ->
        LazyColumn(
            modifier = Modifier.fillMaxHeight().overScrollVertical().nestedScroll(scroll.nestedScrollConnection),
            contentPadding = padding,
        ) {
            item {
                Text(
                    text = stringResource(R.string.rule_templates_hint),
                    fontSize = MiuixTheme.textStyles.footnote1.fontSize,
                    color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                    modifier = Modifier.padding(horizontal = 24.dp, vertical = 8.dp),
                )
            }
            val builtin = templates.filter { it.builtin }
            val user = templates.filterNot { it.builtin }
            item { SmallTitle(stringResource(R.string.template_builtin)) }
            items(builtin, key = { it.id }) { t ->
                TemplateCard(t, profiles.count { it.templateId == t.id }, onClick = { onEdit(t.id) }, onLongPress = { onEdit(t.id) })
            }
            item { SmallTitle(stringResource(R.string.template_user)) }
            if (user.isEmpty()) {
                item {
                    Text(
                        text = stringResource(R.string.template_user_empty),
                        color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                        modifier = Modifier.padding(horizontal = 24.dp, vertical = 8.dp),
                    )
                }
            }
            items(user, key = { it.id }) { t ->
                TemplateCard(t, profiles.count { it.templateId == t.id }, onClick = { onEdit(t.id) }, onLongPress = { actionsFor = t })
            }
            item { Spacer(Modifier.padding(bottom = 12.dp)) }
        }
    }

    val target = actionsFor
    OverlayDialog(show = target != null, title = target?.name, onDismissRequest = { actionsFor = null }) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            TextButton(
                text = stringResource(R.string.action_edit),
                onClick = { actionsFor = null; target?.let { onEdit(it.id) } },
                modifier = Modifier.fillMaxWidth(),
            )
            TextButton(
                text = stringResource(R.string.action_delete),
                onClick = {
                    actionsFor = null
                    target?.let {
                        TemplateStore.delete(it.id)
                        Toast.makeText(context, R.string.template_deleted, Toast.LENGTH_SHORT).show()
                    }
                },
                modifier = Modifier.fillMaxWidth(),
            )
        }
    }

    AddTemplateDialog(show = showAdd, onDismiss = { showAdd = false }, onCreate = { showAdd = false; onEdit(null) })
}

@Composable
private fun TemplateCard(t: RuleTemplate, usedBy: Int, onClick: () -> Unit, onLongPress: () -> Unit) {
    Card(
        modifier = Modifier.padding(horizontal = 12.dp).padding(bottom = 12.dp),
        onClick = onClick,
        onLongPress = onLongPress,
        insideMargin = PaddingValues(16.dp),
    ) {
        Text(t.name, fontWeight = FontWeight.SemiBold)
        val summary = listOfNotNull(
            t.description.ifBlank { null },
            t.sourceUrl.ifBlank { null },
            if (usedBy > 0) stringResource(R.string.template_used_by, usedBy) else null,
        ).joinToString(" · ")
        if (summary.isNotBlank()) {
            Text(
                text = summary,
                fontSize = MiuixTheme.textStyles.footnote1.fontSize,
                color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                modifier = Modifier.padding(top = 2.dp),
            )
        }
    }
}

@Composable
private fun AddTemplateDialog(show: Boolean, onDismiss: () -> Unit, onCreate: () -> Unit) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var kind by remember { mutableIntStateOf(0) } // 0 new, 1 URL, 2 file
    var name by remember { mutableStateOf("") }
    var url by remember { mutableStateOf("") }
    var working by remember { mutableStateOf(false) }

    fun toast(msg: String) = Toast.makeText(context, msg, Toast.LENGTH_LONG).show()

    fun finish(block: suspend () -> RuleTemplate) {
        working = true
        scope.launch {
            runCatching { block() }
                .onSuccess {
                    toast(context.getString(R.string.template_imported, it.name))
                    name = ""; url = ""
                    onDismiss()
                }
                .onFailure { toast(context.getString(R.string.template_invalid, it.message ?: "")) }
            working = false
        }
    }

    val picker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri: Uri? ->
        uri ?: return@rememberLauncherForActivityResult
        finish {
            val text = withContext(Dispatchers.IO) {
                context.contentResolver.openInputStream(uri)?.use { it.readBytes() }?.toString(Charsets.UTF_8) ?: error("cannot read file")
            }
            val fallback = uri.lastPathSegment?.substringAfterLast('/')?.substringBeforeLast('.') ?: "Imported"
            withContext(Dispatchers.IO) { TemplateStore.save(null, name.ifBlank { fallback }, text) }
        }
    }

    OverlayDialog(show = show, title = stringResource(R.string.template_add), onDismissRequest = { if (!working) onDismiss() }) {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            TabRowWithContour(
                tabs = listOf(stringResource(R.string.template_new), stringResource(R.string.add_by_url), stringResource(R.string.add_by_file)),
                selectedTabIndex = kind,
                onTabSelected = { kind = it },
            )
            when (kind) {
                0 -> Text(stringResource(R.string.template_new_hint), color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
                1 -> {
                    TextField(value = name, onValueChange = { name = it }, label = stringResource(R.string.profile_name), singleLine = true)
                    TextField(
                        value = url,
                        onValueChange = { url = it },
                        label = stringResource(R.string.template_url),
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
                    )
                }
                else -> {
                    TextField(value = name, onValueChange = { name = it }, label = stringResource(R.string.profile_name), singleLine = true)
                    Text(stringResource(R.string.template_file_hint), color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
                }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically) {
                TextButton(text = stringResource(android.R.string.cancel), onClick = onDismiss, enabled = !working, modifier = Modifier.weight(1f))
                if (working) {
                    Box(Modifier.width(48.dp), contentAlignment = Alignment.Center) { InfiniteProgressIndicator() }
                }
                TextButton(
                    text = stringResource(
                        when (kind) {
                            0 -> R.string.template_new
                            1 -> R.string.action_add
                            else -> R.string.choose_file
                        },
                    ),
                    enabled = !working,
                    colors = ButtonDefaults.textButtonColorsPrimary(),
                    modifier = Modifier.weight(1f),
                    onClick = {
                        when (kind) {
                            0 -> onCreate()
                            1 -> if (url.isBlank()) toast(context.getString(R.string.error_url_empty)) else finish { TemplateStore.importUrl(name, url) }
                            else -> picker.launch(arrayOf("*/*"))
                        }
                    },
                )
            }
        }
    }
}

/** Edits a user template, or shows a built-in one read-only with "duplicate". */
@Composable
fun TemplateEditScreen(templateId: String?, onBack: () -> Unit, onOpen: (String) -> Unit) {
    val context = LocalContext.current
    val existing = remember(templateId) { templateId?.let { TemplateStore.get(it) } }
    if (templateId != null && existing == null) {
        LaunchedEffect(Unit) { onBack() }
        return
    }
    val readOnly = existing?.builtin == true
    var name by remember(templateId) { mutableStateOf(existing?.name ?: "") }
    var content by remember(templateId) { mutableStateOf(existing?.content ?: NEW_TEMPLATE) }
    val scroll = MiuixScrollBehavior()
    val mono = TextStyle(fontFamily = FontFamily.Monospace, fontSize = 12.sp, color = MiuixTheme.colorScheme.onSurface)

    fun toast(msg: String) = Toast.makeText(context, msg, Toast.LENGTH_LONG).show()

    Scaffold(
        topBar = {
            TopAppBar(
                title = existing?.name ?: stringResource(R.string.template_new),
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
                Column(Modifier.padding(horizontal = 12.dp, vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    if (readOnly) {
                        Text(
                            text = stringResource(R.string.template_builtin_readonly),
                            fontSize = MiuixTheme.textStyles.footnote1.fontSize,
                            color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                        )
                        Card(insideMargin = PaddingValues(12.dp)) { Text(text = content, style = mono) }
                        TextButton(
                            text = stringResource(R.string.template_duplicate),
                            colors = ButtonDefaults.textButtonColorsPrimary(),
                            modifier = Modifier.fillMaxWidth(),
                            onClick = {
                                runCatching { TemplateStore.save(null, context.getString(R.string.template_copy_of, existing.name), existing.content) }
                                    .onSuccess { onOpen(it.id) }
                                    .onFailure { toast(context.getString(R.string.template_invalid, it.message ?: "")) }
                            },
                        )
                    } else {
                        TextField(value = name, onValueChange = { name = it }, label = stringResource(R.string.profile_name), singleLine = true)
                        TextField(
                            value = content,
                            onValueChange = { content = it },
                            textStyle = mono,
                            modifier = Modifier.fillMaxWidth().heightIn(min = 360.dp),
                        )
                        TextButton(
                            text = stringResource(R.string.action_save),
                            colors = ButtonDefaults.textButtonColorsPrimary(),
                            modifier = Modifier.fillMaxWidth(),
                            onClick = {
                                runCatching { TemplateStore.save(existing?.id, name, content) }
                                    .onSuccess {
                                        toast(context.getString(R.string.template_saved))
                                        if (existing == null) onOpen(it.id)
                                    }
                                    .onFailure { toast(context.getString(R.string.template_invalid, it.message ?: "")) }
                            },
                        )
                        Text(
                            text = stringResource(R.string.template_syntax_help),
                            fontSize = MiuixTheme.textStyles.footnote1.fontSize,
                            color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                        )
                    }
                }
            }
        }
    }
}

private val NEW_TEMPLATE = """
    # Clash/mihomo YAML; Surge [Proxy Group] / [Rule] snippets also work.
    proxy-groups:
      - name: 节点选择
        type: select
        proxies: [自动选择, DIRECT]
        include-all: true
      - name: 自动选择
        type: url-test
        include-all: true
        exclude-filter: '剩余|到期|官网'
    rules:
      - GEOSITE,cn,DIRECT
      - GEOIP,CN,DIRECT
      - MATCH,节点选择
""".trimIndent() + "\n"
