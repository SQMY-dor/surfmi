// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.theme.MiuixTheme

/** Delay colour: green < 300 ms, orange < 800 ms, red otherwise / failed. */
fun delayColor(delay: Int): Color = when {
    delay <= 0 -> Color(0xFF9E9E9E)
    delay < 300 -> Color(0xFF2E9E5B)
    delay < 800 -> Color(0xFFE69A28)
    else -> Color(0xFFE5484D)
}

fun delayText(delay: Int): String = when {
    delay == 0 -> "--"
    delay < 0 -> "timeout"
    else -> "$delay ms"
}

@Composable
fun StatBlock(label: String, value: String, modifier: Modifier = Modifier) {
    Column(modifier = modifier, verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(text = label, fontSize = MiuixTheme.textStyles.footnote1.fontSize, color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
        Text(text = value, fontSize = MiuixTheme.textStyles.title4.fontSize, fontWeight = FontWeight.SemiBold)
    }
}

@Composable
fun KeyValueRow(key: String, value: String) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(text = key, color = MiuixTheme.colorScheme.onSurfaceVariantSummary)
        Text(text = value)
    }
}

@Composable
fun VSpace(h: Int = 12) = Spacer(Modifier.height(h.dp))
