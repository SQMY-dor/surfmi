// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !(android && cmfa)

package libcore

// UpdateSystemDNS is a no-op outside Android `cmfa` builds, where
// mihomo reads the system resolver itself.
func UpdateSystemDNS(servers string) {}
