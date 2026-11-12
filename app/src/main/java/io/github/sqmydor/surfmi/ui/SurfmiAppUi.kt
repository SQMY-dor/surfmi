// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi.ui

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.MutableState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.res.stringResource
import io.github.sqmydor.surfmi.R
import io.github.sqmydor.surfmi.ui.screens.ActivityScreen
import io.github.sqmydor.surfmi.ui.screens.AppListScreen
import io.github.sqmydor.surfmi.ui.screens.HomeScreen
import io.github.sqmydor.surfmi.ui.screens.PolicyScreen
import io.github.sqmydor.surfmi.ui.screens.ProfileEditScreen
import io.github.sqmydor.surfmi.ui.screens.ProfilesScreen
import io.github.sqmydor.surfmi.ui.screens.SettingsScreen
import io.github.sqmydor.surfmi.ui.screens.TemplateEditScreen
import io.github.sqmydor.surfmi.ui.screens.TemplatesScreen
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.NavigationBar
import top.yukonga.miuix.kmp.basic.NavigationBarItem
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.ScrollBehavior
import top.yukonga.miuix.kmp.basic.TopAppBar
import top.yukonga.miuix.kmp.icon.MiuixIcons
import top.yukonga.miuix.kmp.icon.extended.Add
import top.yukonga.miuix.kmp.icon.extended.Delete
import top.yukonga.miuix.kmp.icon.extended.Home
import top.yukonga.miuix.kmp.icon.extended.Layers
import top.yukonga.miuix.kmp.icon.extended.ListView
import top.yukonga.miuix.kmp.icon.extended.Notes
import top.yukonga.miuix.kmp.icon.extended.Settings
import top.yukonga.miuix.kmp.icon.extended.Timer

/** Secondary full-screen pages pushed on top of the tab scaffold. */
sealed interface SubPage {
    data object AppList : SubPage
    data class EditProfile(val id: String) : SubPage
    /** [back] is where the template list returns to. */
    data class Templates(val back: SubPage?) : SubPage
    /** id null = new template. */
    data class EditTemplate(val id: String?, val back: SubPage?) : SubPage
}

/** Per-tab hoisted signals that top-bar actions send to the tab content. */
class TabSignals {
    var addProfile by mutableStateOf(false)
    var testAll by mutableIntStateOf(0)
    var clearActivity by mutableIntStateOf(0)
}

@Composable
fun SurfmiAppUi(
    onConnect: () -> Unit,
    onDisconnect: () -> Unit,
    pendingInstallUrl: MutableState<String?>,
) {
    var tab by rememberSaveable { mutableIntStateOf(0) }
    var sub by remember { mutableStateOf<SubPage?>(null) }
    val signals = remember { TabSignals() }

    LaunchedEffect(pendingInstallUrl.value) {
        if (pendingInstallUrl.value != null) {
            tab = 2
            signals.addProfile = true
        }
    }

    when (val page = sub) {
        SubPage.AppList -> {
            BackHandler { sub = null }
            AppListScreen(onBack = { sub = null })
            return
        }
        is SubPage.EditProfile -> {
            BackHandler { sub = null }
            ProfileEditScreen(profileId = page.id, onBack = { sub = null }, onOpenTemplates = { sub = SubPage.Templates(page) })
            return
        }
        is SubPage.Templates -> {
            BackHandler { sub = page.back }
            TemplatesScreen(onBack = { sub = page.back }, onEdit = { sub = SubPage.EditTemplate(it, page) })
            return
        }
        is SubPage.EditTemplate -> {
            BackHandler { sub = page.back }
            TemplateEditScreen(
                templateId = page.id,
                onBack = { sub = page.back },
                onOpen = { sub = SubPage.EditTemplate(it, page.back) },
            )
            return
        }
        null -> Unit
    }

    BackHandler(enabled = tab != 0) { tab = 0 }

    val titles = listOf(
        stringResource(R.string.tab_home),
        stringResource(R.string.tab_policy),
        stringResource(R.string.tab_profiles),
        stringResource(R.string.tab_activity),
        stringResource(R.string.tab_settings),
    )
    val icons = listOf(MiuixIcons.Home, MiuixIcons.Layers, MiuixIcons.ListView, MiuixIcons.Notes, MiuixIcons.Settings)
    val behaviors: List<ScrollBehavior> = listOf(
        MiuixScrollBehavior(), MiuixScrollBehavior(), MiuixScrollBehavior(), MiuixScrollBehavior(), MiuixScrollBehavior(),
    )

    Scaffold(
        topBar = {
            TopAppBar(
                title = titles[tab],
                scrollBehavior = behaviors[tab],
                actions = {
                    when (tab) {
                        1 -> IconButton(onClick = { signals.testAll++ }) {
                            Icon(MiuixIcons.Timer, contentDescription = stringResource(R.string.action_test_latency))
                        }
                        2 -> IconButton(onClick = { signals.addProfile = true }) {
                            Icon(MiuixIcons.Add, contentDescription = stringResource(R.string.action_add_profile))
                        }
                        3 -> IconButton(onClick = { signals.clearActivity++ }) {
                            Icon(MiuixIcons.Delete, contentDescription = stringResource(R.string.action_clear))
                        }
                    }
                },
            )
        },
        bottomBar = {
            NavigationBar {
                titles.forEachIndexed { i, title ->
                    NavigationBarItem(selected = tab == i, onClick = { tab = i }, icon = icons[i], label = title)
                }
            }
        },
    ) { padding: PaddingValues ->
        when (tab) {
            0 -> HomeScreen(padding, behaviors[0], onConnect, onDisconnect, onOpenProfiles = { tab = 2 })
            1 -> PolicyScreen(padding, behaviors[1], signals.testAll)
            2 -> ProfilesScreen(
                padding = padding,
                scrollBehavior = behaviors[2],
                showAdd = signals.addProfile,
                initialUrl = pendingInstallUrl.value,
                onAddDismiss = {
                    signals.addProfile = false
                    pendingInstallUrl.value = null
                },
                onEdit = { sub = SubPage.EditProfile(it) },
            )
            3 -> ActivityScreen(padding, behaviors[3], signals.clearActivity)
            else -> SettingsScreen(
                padding,
                behaviors[4],
                onOpenAppList = { sub = SubPage.AppList },
                onOpenTemplates = { sub = SubPage.Templates(null) },
            )
        }
    }
}
