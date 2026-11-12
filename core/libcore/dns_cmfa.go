// SPDX-License-Identifier: GPL-3.0-or-later

//go:build android && cmfa

package libcore

import (
	"strings"

	"github.com/metacubex/mihomo/dns"
)

// UpdateSystemDNS tells mihomo which DNS servers the underlying (non-VPN)
// network uses; they back the "system" nameserver. Comma separated.
func UpdateSystemDNS(servers string) {
	var list []string
	for _, s := range strings.Split(servers, ",") {
		if s = strings.TrimSpace(s); s != "" {
			list = append(list, s)
		}
	}
	dns.UpdateSystemDNS(list)
	dns.FlushCacheWithDefaultResolver()
}
