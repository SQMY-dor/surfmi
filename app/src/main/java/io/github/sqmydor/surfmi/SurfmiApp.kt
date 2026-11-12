// SPDX-License-Identifier: GPL-3.0-or-later
package io.github.sqmydor.surfmi

import android.app.Application
import io.github.sqmydor.surfmi.data.AutoUpdateWorker
import io.github.sqmydor.surfmi.data.ProfileStore
import io.github.sqmydor.surfmi.data.Settings
import io.github.sqmydor.surfmi.data.TemplateStore
import io.github.sqmydor.surfmi.service.SurfmiVpnService

class SurfmiApp : Application() {
    override fun onCreate() {
        super.onCreate()
        Settings.init(this)
        ProfileStore.init(this)
        TemplateStore.init(this)
        SurfmiVpnService.ensureChannel(this)
        AutoUpdateWorker.schedule(this)
    }
}
