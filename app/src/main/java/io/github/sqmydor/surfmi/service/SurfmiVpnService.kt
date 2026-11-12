// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.content.pm.ServiceInfo
import android.net.ConnectivityManager
import android.net.IpPrefix
import android.net.LinkProperties
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.system.OsConstants
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import io.github.sqmydor.surfmi.BuildConfig
import io.github.sqmydor.surfmi.R
import io.github.sqmydor.surfmi.core.Core
import io.github.sqmydor.surfmi.core.libcore.Libcore
import io.github.sqmydor.surfmi.core.libcore.Platform
import io.github.sqmydor.surfmi.data.PerAppMode
import io.github.sqmydor.surfmi.data.ProfileStore
import io.github.sqmydor.surfmi.data.Settings
import io.github.sqmydor.surfmi.ui.MainActivity
import io.github.sqmydor.surfmi.util.formatSpeed
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import java.net.InetAddress
import java.net.InetSocketAddress

/**
 * Owns the TUN device. The fd from [VpnService.Builder.establish] is passed to
 * mihomo (sing-tun, gVisor/system stack); mihomo's own sockets are protected
 * so they bypass the tunnel.
 */
class SurfmiVpnService : VpnService(), Platform {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private var tun: ParcelFileDescriptor? = null
    private var ticker: Job? = null
    private var networkCallback: ConnectivityManager.NetworkCallback? = null
    private val connectivity by lazy { getSystemService(ConnectivityManager::class.java) }
    private val notifications by lazy { getSystemService(NotificationManager::class.java) }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_STOP -> stopVpn()
            else -> startVpn()
        }
        return START_NOT_STICKY
    }

    override fun onRevoke() = stopVpn()

    override fun onDestroy() {
        stopCore()
        scope.cancel()
        super.onDestroy()
    }

    private fun startVpn() {
        if (ServiceState.state.value == VpnState.Running || ServiceState.state.value == VpnState.Starting) return
        ServiceState.set(VpnState.Starting)
        ServiceState.setError(null)
        startForegroundCompat(buildNotification(getString(R.string.notification_connecting)))
        scope.launch {
            try {
                val profile = ProfileStore.active() ?: error(getString(R.string.error_no_profile))
                val config = ProfileStore.configFile(profile)
                if (!config.exists()) error(getString(R.string.error_profile_missing))

                Libcore.setPlatform(this@SurfmiVpnService)
                Libcore.setLogLevel(Settings.logLevel.value)
                val fd = establish(config.absolutePath) ?: error(getString(R.string.error_vpn_permission))
                tun = fd

                val opts = Libcore.newStartOptions()
                opts.setTunFd(fd.fd)
                opts.setStack(Settings.stack.value)
                opts.setMTU(Settings.mtu.value)
                opts.setIPv6(Settings.ipv6.value)
                opts.setMode(Settings.mode.value)
                opts.setLogLevel(Settings.logLevel.value)
                opts.setTestURL(Settings.testUrl.value)
                opts.setDNSServers(Settings.dnsServers.value)
                Libcore.start(config.absolutePath, ProfileStore.homeDir.absolutePath, opts)

                watchNetwork()
                ServiceState.set(VpnState.Running)
                ToggleTileService.refresh(this@SurfmiVpnService)
                startTicker(profile.name)
            } catch (t: Throwable) {
                LogBuffer.add("error", "start failed: ${t.message}")
                ServiceState.setError(t.message ?: t.javaClass.simpleName)
                stopVpn()
            }
        }
    }

    private fun establish(configPath: String): ParcelFileDescriptor? {
        val (addr4, prefix4) = Libcore.tunInet4(configPath).split("/").let { it[0] to it[1].toInt() }
        val b = Builder()
            .setSession(BuildConfig.APP_NAME)
            .setMtu(Settings.mtu.value)
            .addAddress(addr4, prefix4)
            .addDnsServer(nextAddress(addr4))
            .setBlocking(false)
        b.addRoute("0.0.0.0", 0)
        if (Settings.bypassLan.value && Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            LAN_ROUTES_V4.forEach { (a, p) -> b.excludeRoute(IpPrefix(InetAddress.getByName(a), p)) }
        }
        if (Settings.ipv6.value) {
            val (addr6, prefix6) = Libcore.TunInet6.split("/").let { it[0] to it[1].toInt() }
            b.addAddress(addr6, prefix6)
            b.addRoute("::", 0)
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) b.setMetered(false)

        val self = packageName
        val pkgs = Settings.perAppPackages.value - self
        when (Settings.perAppMode.value) {
            PerAppMode.ALLOW -> pkgs.forEach { runCatching { b.addAllowedApplication(it) } }
            PerAppMode.BYPASS -> {
                pkgs.forEach { runCatching { b.addDisallowedApplication(it) } }
                b.addDisallowedApplication(self)
            }
            // Our own traffic (subscription updates, mihomo sockets) never loops through the TUN.
            else -> b.addDisallowedApplication(self)
        }
        b.setConfigureIntent(
            PendingIntent.getActivity(this, 0, Intent(this, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE),
        )
        return b.establish()
    }

    private fun nextAddress(ip: String): String {
        val bytes = InetAddress.getByName(ip).address
        bytes[bytes.size - 1] = (bytes[bytes.size - 1] + 1).toByte()
        return InetAddress.getByAddress(bytes).hostAddress ?: ip
    }

    private fun watchNetwork() {
        val cb = object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: Network) {
                setUnderlyingNetworks(arrayOf(network))
            }

            override fun onLinkPropertiesChanged(network: Network, lp: LinkProperties) {
                val servers = lp.dnsServers.mapNotNull { it.hostAddress }.joinToString(",")
                Libcore.updateSystemDNS(servers)
            }

            override fun onLost(network: Network) {
                setUnderlyingNetworks(null)
            }
        }
        val req = NetworkRequest.Builder()
            .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
            .addCapability(NetworkCapabilities.NET_CAPABILITY_NOT_VPN)
            .build()
        runCatching { connectivity.requestNetwork(req, cb) }
        networkCallback = cb
    }

    private fun startTicker(profileName: String) {
        ticker?.cancel()
        ticker = scope.launch {
            while (isActive) {
                val t = Core.traffic()
                ServiceState.setTraffic(t)
                if (hasNotificationPermission()) {
                    notifications.notify(
                        NOTIFICATION_ID,
                        buildNotification("↑ ${formatSpeed(t.up)}   ↓ ${formatSpeed(t.down)}", profileName),
                    )
                }
                delay(1000)
            }
        }
    }

    private fun hasNotificationPermission() = Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU ||
        ContextCompat.checkSelfPermission(this, android.Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED

    private fun stopCore() {
        ticker?.cancel()
        ticker = null
        networkCallback?.let { runCatching { connectivity.unregisterNetworkCallback(it) } }
        networkCallback = null
        runCatching { Libcore.stop() }
        runCatching { tun?.close() }
        tun = null
    }

    private fun stopVpn() {
        if (ServiceState.state.value != VpnState.Stopped) ServiceState.set(VpnState.Stopping)
        stopCore()
        ServiceState.set(VpnState.Stopped)
        ToggleTileService.refresh(this)
        ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    // ---------------------------------------------------------------- Platform (called from Go)

    override fun protect(fd: Int): Boolean = super.protect(fd)

    override fun findPackage(network: String, srcIP: String, srcPort: Int, dstIP: String, dstPort: Int): String {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.Q) return ""
        return try {
            val proto = if (network.startsWith("udp")) OsConstants.IPPROTO_UDP else OsConstants.IPPROTO_TCP
            val uid = connectivity.getConnectionOwnerUid(
                proto,
                InetSocketAddress(InetAddress.getByName(srcIP), srcPort),
                InetSocketAddress(InetAddress.getByName(dstIP), dstPort),
            )
            if (uid < 0) "" else packageManager.getPackagesForUid(uid)?.firstOrNull() ?: ""
        } catch (_: Exception) {
            ""
        }
    }

    override fun onLog(level: String, message: String) = LogBuffer.add(level, message)

    // ---------------------------------------------------------------- notification

    private fun startForegroundCompat(n: Notification) {
        val type = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE
        } else {
            0
        }
        ServiceCompat.startForeground(this, NOTIFICATION_ID, n, type)
    }

    private fun buildNotification(text: String, title: String = BuildConfig.APP_NAME): Notification {
        ensureChannel(this)
        val open = PendingIntent.getActivity(
            this, 0, Intent(this, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        val stop = PendingIntent.getService(
            this, 1, Intent(this, SurfmiVpnService::class.java).setAction(ACTION_STOP), PendingIntent.FLAG_IMMUTABLE,
        )
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(title)
            .setContentText(text)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setShowWhen(false)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .setContentIntent(open)
            .addAction(0, getString(R.string.action_disconnect), stop)
            .build()
    }

    companion object {
        const val ACTION_START = "io.github.sqmydor.surfmi.START"
        const val ACTION_STOP = "io.github.sqmydor.surfmi.STOP"
        private const val CHANNEL_ID = "vpn"
        private const val NOTIFICATION_ID = 1

        private val LAN_ROUTES_V4 = listOf(
            "10.0.0.0" to 8, "100.64.0.0" to 10, "169.254.0.0" to 16, "172.16.0.0" to 12, "192.168.0.0" to 16,
        )

        fun ensureChannel(context: Context) {
            val nm = context.getSystemService(NotificationManager::class.java)
            if (nm.getNotificationChannel(CHANNEL_ID) == null) {
                nm.createNotificationChannel(
                    NotificationChannel(CHANNEL_ID, context.getString(R.string.notification_channel), NotificationManager.IMPORTANCE_LOW),
                )
            }
        }
    }
}
