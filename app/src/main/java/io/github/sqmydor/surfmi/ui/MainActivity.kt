// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi.ui

import android.Manifest
import android.content.Intent
import android.os.Build
import android.os.Bundle
import android.widget.Toast
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.compose.runtime.mutableStateOf
import io.github.sqmydor.surfmi.R
import io.github.sqmydor.surfmi.data.ProfileStore
import io.github.sqmydor.surfmi.service.VpnController
import io.github.sqmydor.surfmi.ui.theme.SurfmiTheme

class MainActivity : AppCompatActivity() {
    /** A subscription URL received through an install-config deep link. */
    private val pendingInstallUrl = mutableStateOf<String?>(null)

    private val vpnConsent = registerForActivityResult(ActivityResultContracts.StartActivityForResult()) { res ->
        if (res.resultCode == RESULT_OK) VpnController.start(this)
        else Toast.makeText(this, R.string.error_vpn_permission, Toast.LENGTH_SHORT).show()
    }

    private val notificationPermission = registerForActivityResult(ActivityResultContracts.RequestPermission()) { }

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        handleIntent(intent)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
        }
        setContent {
            SurfmiTheme {
                SurfmiAppUi(
                    onConnect = ::connect,
                    onDisconnect = { VpnController.stop(this) },
                    pendingInstallUrl = pendingInstallUrl,
                )
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleIntent(intent)
    }

    private fun handleIntent(intent: Intent?) {
        intent ?: return
        if (intent.action == ACTION_CONNECT) {
            connect()
            return
        }
        val data = intent.data ?: return
        if (data.host == "install-config") {
            data.getQueryParameter("url")?.let { pendingInstallUrl.value = it }
        }
    }

    private fun connect() {
        if (ProfileStore.active() == null) {
            Toast.makeText(this, R.string.error_no_profile, Toast.LENGTH_SHORT).show()
            return
        }
        val consent = VpnController.prepareIntent(this)
        if (consent != null) vpnConsent.launch(consent) else VpnController.start(this)
    }

    companion object {
        const val ACTION_CONNECT = "io.github.sqmydor.surfmi.CONNECT"
    }
}
