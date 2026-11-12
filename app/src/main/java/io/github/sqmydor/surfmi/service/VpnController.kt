// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi.service

import android.content.Context
import android.content.Intent
import android.net.VpnService
import androidx.core.content.ContextCompat

object VpnController {
    /** Returns the consent intent if the user still has to approve the VPN. */
    fun prepareIntent(context: Context): Intent? = VpnService.prepare(context)

    fun start(context: Context) {
        val i = Intent(context, SurfmiVpnService::class.java).setAction(SurfmiVpnService.ACTION_START)
        ContextCompat.startForegroundService(context, i)
    }

    fun stop(context: Context) {
        if (ServiceState.state.value == VpnState.Stopped) return
        val i = Intent(context, SurfmiVpnService::class.java).setAction(SurfmiVpnService.ACTION_STOP)
        context.startService(i)
    }

    fun toggle(context: Context) {
        if (ServiceState.state.value == VpnState.Stopped) start(context) else stop(context)
    }
}
