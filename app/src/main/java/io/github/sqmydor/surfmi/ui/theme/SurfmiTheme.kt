// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi.ui.theme

import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.graphics.Color
import io.github.sqmydor.surfmi.data.Settings
import io.github.sqmydor.surfmi.data.ThemeMode
import top.yukonga.miuix.kmp.theme.ColorSchemeMode
import top.yukonga.miuix.kmp.theme.MiuixTheme
import top.yukonga.miuix.kmp.theme.ThemeController

/** MIUI/HyperOS-style theme from miuix; follows the system dark mode by default. */
@Composable
fun SurfmiTheme(content: @Composable () -> Unit) {
    val mode by Settings.themeMode.flow.collectAsState()
    val controller = remember(mode) {
        ThemeController(
            colorSchemeMode = when (mode) {
                ThemeMode.LIGHT -> ColorSchemeMode.Light
                ThemeMode.DARK -> ColorSchemeMode.Dark
                ThemeMode.MONET -> ColorSchemeMode.MonetSystem
                else -> ColorSchemeMode.System
            },
            keyColor = Color(0xFF3482FF),
        )
    }
    MiuixTheme(controller = controller, content = content)
}
