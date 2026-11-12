// SPDX-License-Identifier: GPL-3.0-or-later

package convert

import (
	"strings"
	"testing"

	"github.com/metacubex/mihomo/config"
	C "github.com/metacubex/mihomo/constant"
	_ "github.com/metacubex/mihomo/hub/executor" // provides linknamed config hooks
)

// stripGeoRules removes rules that make mihomo download GeoIP/GeoSite/ASN
// databases while parsing, so validation runs offline.
func stripGeoRules(yml string) string {
	var out []string
	for _, line := range strings.Split(yml, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "- GEOIP,") || strings.HasPrefix(t, "- GEOSITE,") || strings.HasPrefix(t, "- IP-ASN,") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// TestMihomoAcceptsOutput feeds every converted sample through mihomo's own
// config parser, which instantiates every proxy adapter, group and rule.
func TestMihomoAcceptsOutput(t *testing.T) {
	C.SetHomeDir(t.TempDir())
	samples := []string{"surge_full.conf", "uri_plain.txt", "uri_base64.txt", "singbox.json", "clash_with_rules.yaml", "clash_no_rules.yaml", "policy_path_list.txt"}
	for _, s := range samples {
		t.Run(s, func(t *testing.T) {
			res, err := Convert(readTestdata(t, s), Options{Fetcher: testFetcher})
			if err != nil {
				t.Fatal(err)
			}
			raw, err := config.UnmarshalRawConfig([]byte(stripGeoRules(res.YAML)))
			if err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			cfg, err := config.ParseRawConfig(raw)
			if err != nil {
				t.Fatalf("mihomo rejected converted config: %v\n%s", err, res.YAML)
			}
			// +3: DIRECT, REJECT, REJECT-DROP ... built-ins are also in the map
			if len(cfg.Proxies) < res.Proxies+res.Groups {
				t.Errorf("mihomo loaded %d proxies+groups, converter produced %d+%d", len(cfg.Proxies), res.Proxies, res.Groups)
			}
		})
	}
}
