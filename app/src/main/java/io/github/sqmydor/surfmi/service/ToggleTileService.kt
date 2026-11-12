// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi.service

import android.annotation.SuppressLint
import android.app.PendingIntent
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.os.Build
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService
import io.github.sqmydor.surfmi.data.ProfileStore
import io.github.sqmydor.surfmi.ui.MainActivity

/** Quick Settings tile that connects / disconnects the VPN. */
class ToggleTileService : TileService() {
    override fun onStartListening() {
        super.onStartListening()
        update()
    }

    override fun onClick() {
        super.onClick()
        if (ServiceState.state.value == VpnState.Stopped && VpnController.prepareIntent(this) != null) {
            // consent dialog must be shown from an activity
            openApp()
            return
        }
        runCatching { VpnController.toggle(this) }.onFailure { openApp() }
        update()
    }

    @SuppressLint("StartActivityAndCollapseDeprecated")
    private fun openApp() {
        val intent = Intent(this, MainActivity::class.java)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            .setAction(MainActivity.ACTION_CONNECT)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            startActivityAndCollapse(PendingIntent.getActivity(this, 0, intent, PendingIntent.FLAG_IMMUTABLE))
        } else {
            @Suppress("DEPRECATION")
            startActivityAndCollapse(intent)
        }
    }

    private fun update() {
        val tile = qsTile ?: return
        val state = ServiceState.state.value
        tile.state = when (state) {
            VpnState.Running, VpnState.Starting -> Tile.STATE_ACTIVE
            else -> Tile.STATE_INACTIVE
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            tile.subtitle = ProfileStore.active()?.name
        }
        tile.updateTile()
    }

    companion object {
        fun refresh(context: Context) {
            runCatching {
                requestListeningState(context, ComponentName(context, ToggleTileService::class.java))
            }
        }
    }
}
