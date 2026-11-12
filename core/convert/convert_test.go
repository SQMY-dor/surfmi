// SPDX-License-Identifier: GPL-3.0-or-later

package convert

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// testFetcher serves policy-path URLs from testdata.
var testFetcher = FetcherFunc(func(url string) ([]byte, error) {
	if url == "https://sub.example.com/list.txt" {
		return os.ReadFile(filepath.Join("testdata", "policy_path_list.txt"))
	}
	return nil, fmt.Errorf("404 %s", url)
})

type parsed struct {
	Proxies        []map[string]any          `yaml:"proxies"`
	ProxyGroups    []map[string]any          `yaml:"proxy-groups"`
	ProxyProviders map[string]map[string]any `yaml:"proxy-providers"`
	RuleProviders  map[string]map[string]any `yaml:"rule-providers"`
	Rules          []string                  `yaml:"rules"`
	Hosts          map[string]any            `yaml:"hosts"`
	DNS            map[string]any            `yaml:"dns"`
	IPv6           bool                      `yaml:"ipv6"`
	LogLevel       string                    `yaml:"log-level"`
}

func mustConvert(t *testing.T, name string, opt Options) (*Result, parsed) {
	t.Helper()
	res, err := Convert(readTestdata(t, name), opt)
	if err != nil {
		t.Fatalf("convert %s: %v", name, err)
	}
	var p parsed
	if err := yaml.Unmarshal([]byte(res.YAML), &p); err != nil {
		t.Fatalf("output of %s is not valid yaml: %v\n%s", name, err, res.YAML)
	}
	return res, p
}

func findProxy(p parsed, name string) map[string]any {
	for _, px := range p.Proxies {
		if px["name"] == name {
			return px
		}
	}
	return nil
}

func findGroup(p parsed, name string) map[string]any {
	for _, g := range p.ProxyGroups {
		if g["name"] == name {
			return g
		}
	}
	return nil
}

func hasRule(p parsed, rule string) bool {
	for _, r := range p.Rules {
		if r == rule {
			return true
		}
	}
	return false
}

func hasWarning(res *Result, substr string) bool {
	for _, w := range res.Warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

func TestDetect(t *testing.T) {
	cases := map[string]Format{
		"surge_full.conf":       FormatSurge,
		"policy_path_list.txt":  FormatSurgeList,
		"clash_with_rules.yaml": FormatClash,
		"clash_no_rules.yaml":   FormatClash,
		"singbox.json":          FormatSingBox,
		"uri_plain.txt":         FormatURIList,
		"uri_base64.txt":        FormatURIList,
	}
	for file, want := range cases {
		if got := Detect(readTestdata(t, file)); got != want {
			t.Errorf("Detect(%s) = %s, want %s", file, got, want)
		}
	}
	if Detect([]byte("hello world")) != FormatUnknown {
		t.Error("plain text should be unknown")
	}
}

func TestSurgeProxies(t *testing.T) {
	res, p := mustConvert(t, "surge_full.conf", Options{Fetcher: testFetcher})

	ss := findProxy(p, "HK-SS")
	if ss["type"] != "ss" || ss["cipher"] != "aes-256-gcm" || ss["udp"] != true || ss["tfo"] != true {
		t.Errorf("HK-SS mapped wrong: %v", ss)
	}
	obfs := findProxy(p, "HK-SS-Obfs")
	if obfs["plugin"] != "obfs" || obfs["plugin-opts"].(map[string]any)["host"] != "bing.com" {
		t.Errorf("obfs mapped wrong: %v", obfs)
	}
	vm := findProxy(p, "JP-VMess")
	if vm["uuid"] == nil || vm["network"] != "ws" || vm["tls"] != true || vm["servername"] != "jp.example.com" {
		t.Errorf("vmess mapped wrong: %v", vm)
	}
	hdr := vm["ws-opts"].(map[string]any)["headers"].(map[string]any)
	if hdr["Host"] != "jp.example.com" || hdr["User-Agent"] != "Mozilla" {
		t.Errorf("ws headers wrong: %v", hdr)
	}
	if tr := findProxy(p, "US-Trojan"); tr["skip-cert-verify"] != true || tr["sni"] != "us.example.com" {
		t.Errorf("trojan wrong: %v", tr)
	}
	sn := findProxy(p, "SG-Snell")
	if sn["version"] != 4 || sn["psk"] != "snellpsk" || sn["obfs-opts"].(map[string]any)["mode"] != "tls" || sn["reuse"] != true {
		t.Errorf("snell wrong: %v", sn)
	}
	if h := findProxy(p, "TW-HTTP"); h["type"] != "http" || h["tls"] != true || h["username"] != "user1" {
		t.Errorf("https wrong: %v", h)
	}
	if s := findProxy(p, "KR-SOCKS"); s["type"] != "socks5" || s["username"] != "u" {
		t.Errorf("socks5 wrong: %v", s)
	}
	if tu := findProxy(p, "DE-TUIC"); tu["type"] != "tuic" || tu["password"] != "tuicpass" || tu["uuid"] == nil {
		t.Errorf("tuic wrong: %v", tu)
	}
	hy := findProxy(p, "FR-Hy2")
	if hy["down"] != "200 Mbps" || hy["obfs"] != "salamander" || hy["obfs-password"] != "obfspw" {
		t.Errorf("hy2 wrong: %v", hy)
	}
	wg := findProxy(p, "NL-WG")
	if wg["type"] != "wireguard" || wg["server"] != "nl.example.com" || wg["port"] != 51820 ||
		wg["ip"] != "10.8.0.2" || wg["dialer-proxy"] != "HK-SS" || fmt.Sprint(wg["reserved"]) != "[1 2 3]" {
		t.Errorf("wireguard wrong: %v", wg)
	}
	if a := findProxy(p, "UK-AnyTLS"); a["type"] != "anytls" {
		t.Errorf("anytls wrong: %v", a)
	}
	if findProxy(p, "Ext") != nil || !hasWarning(res, `"Ext" skipped`) {
		t.Error("external proxy should be skipped with a warning")
	}
	if d := findProxy(p, "On"); d["type"] != "direct" {
		t.Errorf("direct policy wrong: %v", d)
	}
}

func TestSurgeGroups(t *testing.T) {
	_, p := mustConvert(t, "surge_full.conf", Options{Fetcher: testFetcher})

	proxy := findGroup(p, "Proxy")
	if proxy["type"] != "select" || !strings.Contains(fmt.Sprint(proxy["proxies"]), "Streaming") {
		t.Errorf("Proxy group / include-other-group wrong: %v", proxy)
	}
	auto := findGroup(p, "Auto")
	if auto["type"] != "url-test" || auto["tolerance"] != 50 || auto["timeout"] != 5000 || auto["interval"] != 600 {
		t.Errorf("url-test wrong: %v", auto)
	}
	if fb := findGroup(p, "Fallback"); fb["type"] != "fallback" || fb["url"] != "http://cp.cloudflare.com/generate_204" {
		t.Errorf("fallback should use proxy-test-url: %v", fb)
	}
	if lb := findGroup(p, "LB"); lb["strategy"] != "consistent-hashing" {
		t.Errorf("load-balance persistent wrong: %v", lb)
	}
	st := findGroup(p, "Streaming")
	if st["include-all-proxies"] != true || st["filter"] != "(JP|US)" {
		t.Errorf("include-all-proxies wrong: %v", st)
	}
	home := findGroup(p, "Home")
	if home["type"] != "select" || fmt.Sprint(home["proxies"]) != "[Proxy DIRECT]" {
		t.Errorf("subnet wrong: %v", home)
	}
	remote := findGroup(p, "Remote")
	if fmt.Sprint(remote["proxies"]) != "[R-HK-01 R-JP-02]" {
		t.Errorf("policy-path inline + regex filter wrong: %v", remote)
	}
	if findProxy(p, "R-HK-01") == nil || findProxy(p, "Other-03") != nil {
		t.Error("policy-path proxies not inlined/filtered")
	}
}

func TestSurgePolicyPathWithoutFetcher(t *testing.T) {
	res, p := mustConvert(t, "surge_full.conf", Options{})
	remote := findGroup(p, "Remote")
	if fmt.Sprint(remote["use"]) != "[pp_Remote]" || remote["filter"] != "^R-" {
		t.Errorf("policy-path provider fallback wrong: %v", remote)
	}
	if p.ProxyProviders["pp_Remote"]["url"] != "https://sub.example.com/list.txt" {
		t.Errorf("provider wrong: %v", p.ProxyProviders)
	}
	if !hasWarning(res, "loaded at runtime") {
		t.Error("expected runtime provider warning")
	}
}

func TestSurgeRules(t *testing.T) {
	res, p := mustConvert(t, "surge_full.conf", Options{Fetcher: testFetcher})
	want := []string{
		"DOMAIN,ads.example.com,REJECT",
		"DOMAIN-SUFFIX,google.com,Proxy",
		"DOMAIN-KEYWORD,youtube,Streaming",
		"DOMAIN-WILDCARD,*.example.org,DIRECT",
		"IP-CIDR,1.1.1.1/32,Proxy,no-resolve",
		"IP-CIDR6,2001:db8::/32,Proxy",
		"GEOIP,US,Proxy",
		"IP-ASN,13335,Proxy,no-resolve",
		"PROCESS-NAME,com.android.chrome,Proxy",
		"DST-PORT,22,DIRECT",
		"SRC-IP-CIDR,192.168.1.10/32,DIRECT",
		"DST-PORT,10000-65535,Proxy",
		"NETWORK,udp,Proxy",
		"AND,((DOMAIN-SUFFIX,apple.com),(DST-PORT,443)),DIRECT",
		"OR,((DOMAIN,a.com),(DOMAIN,b.com)),Proxy",
		"NOT,((DOMAIN-SUFFIX,cn)),Proxy",
		"RULE-SET,Telegram,Proxy",
		"RULE-SET,reject-domains,REJECT",
		"IP-CIDR,192.168.0.0/16,DIRECT,no-resolve",
		"GEOIP,CN,DIRECT",
		"MATCH,Proxy",
	}
	for _, r := range want {
		if !hasRule(p, r) {
			t.Errorf("missing rule %q", r)
		}
	}
	if p.Rules[len(p.Rules)-1] != "MATCH,Proxy" {
		t.Errorf("MATCH must be last, got %q", p.Rules[len(p.Rules)-1])
	}
	for _, r := range p.Rules {
		if strings.Contains(r, "after-final") || strings.HasPrefix(r, "USER-AGENT") || strings.HasPrefix(r, "URL-REGEX") {
			t.Errorf("rule %q should have been dropped", r)
		}
	}
	if rp := p.RuleProviders["Telegram"]; rp["behavior"] != "classical" || rp["format"] != "text" {
		t.Errorf("RULE-SET provider wrong: %v", rp)
	}
	if rp := p.RuleProviders["reject-domains"]; rp["behavior"] != "domain" {
		t.Errorf("DOMAIN-SET provider wrong: %v", rp)
	}
	for _, w := range []string{"USER-AGENT", "URL-REGEX", "SYSTEM", "[URL Rewrite] ignored", "[MITM] ignored", "[Script] ignored", "PROCESS-NAME rules match Android package names"} {
		if !hasWarning(res, w) {
			t.Errorf("expected warning containing %q; got %v", w, res.Warnings)
		}
	}
}

func TestSurgeGeneralHostManaged(t *testing.T) {
	res, p := mustConvert(t, "surge_full.conf", Options{Fetcher: testFetcher})
	if p.IPv6 || p.LogLevel != "info" {
		t.Errorf("general wrong: ipv6=%v log=%s", p.IPv6, p.LogLevel)
	}
	if fmt.Sprint(p.DNS["default-nameserver"]) != "[223.5.5.5 119.29.29.29]" ||
		fmt.Sprint(p.DNS["nameserver"]) != "[https://dns.alidns.com/dns-query]" {
		t.Errorf("dns wrong: %v", p.DNS)
	}
	if !strings.Contains(fmt.Sprint(p.DNS["fake-ip-filter"]), "*.srv.nintendo.net") {
		t.Errorf("always-real-ip not mapped: %v", p.DNS["fake-ip-filter"])
	}
	if p.Hosts["router.lan"] != "192.168.1.1" {
		t.Errorf("hosts wrong: %v", p.Hosts)
	}
	if pol := p.DNS["nameserver-policy"].(map[string]any); pol["+.corp.example.com"] != "10.0.0.53" {
		t.Errorf("nameserver-policy wrong: %v", pol)
	}
	if res.Managed == nil || res.Managed.URL != "https://example.com/surge.conf" || res.Managed.Interval != 43200 {
		t.Errorf("managed wrong: %+v", res.Managed)
	}
}

func TestURIList(t *testing.T) {
	for _, f := range []string{"uri_plain.txt", "uri_base64.txt"} {
		res, p := mustConvert(t, f, Options{})
		if res.Proxies != 11 {
			t.Errorf("%s: want 11 proxies, got %d (%v)", f, res.Proxies, res.Warnings)
		}
		if !hasWarning(res, "ShadowsocksR") || !hasWarning(res, `unsupported scheme "foo"`) {
			t.Errorf("%s: missing skip warnings: %v", f, res.Warnings)
		}
		if px := findProxy(p, "SS Node"); px["cipher"] != "aes-256-gcm" || px["password"] != "sspass" {
			t.Errorf("ss wrong: %v", px)
		}
		if px := findProxy(p, "SS2022"); px["password"] != "AAECAwQFBgcICQoLDA0ODw==" {
			t.Errorf("ss2022 wrong: %v", px)
		}
		if px := findProxy(p, "SS-Obfs"); px["plugin"] != "obfs" {
			t.Errorf("ss obfs wrong: %v", px)
		}
		if px := findProxy(p, "VMess WS"); px["network"] != "ws" || px["servername"] != "vm.example.com" {
			t.Errorf("vmess wrong: %v", px)
		}
		vr := findProxy(p, "VLESS Reality")
		if vr["flow"] != "xtls-rprx-vision" || vr["reality-opts"].(map[string]any)["short-id"] != "0123abcd" || vr["client-fingerprint"] != "chrome" {
			t.Errorf("vless reality wrong: %v", vr)
		}
		if px := findProxy(p, "VLESS-gRPC"); px["network"] != "grpc" {
			t.Errorf("vless grpc wrong: %v", px)
		}
		if px := findProxy(p, "Trojan WS"); px["network"] != "ws" || px["sni"] != "tr.example.com" {
			t.Errorf("trojan wrong: %v", px)
		}
		if px := findProxy(p, "Hy2"); px["obfs"] != "salamander" || px["skip-cert-verify"] != true {
			t.Errorf("hy2 wrong: %v", px)
		}
		if px := findProxy(p, "Hy2-Hop"); px["ports"] != "20000-30000" || px["port"] != 20000 {
			t.Errorf("hy2 hop wrong: %v", px)
		}
		if px := findProxy(p, "TUIC"); px["congestion-controller"] != "bbr" || px["password"] != "tuicpw" {
			t.Errorf("tuic wrong: %v", px)
		}
		// default template
		if findGroup(p, TemplateGroupProxy) == nil || findGroup(p, TemplateGroupAuto) == nil {
			t.Errorf("%s: template groups missing", f)
		}
		if !hasRule(p, "GEOIP,CN,DIRECT") || p.Rules[len(p.Rules)-1] != "MATCH,"+TemplateGroupProxy {
			t.Errorf("%s: template rules wrong: %v", f, p.Rules)
		}
	}
}

func TestSingBox(t *testing.T) {
	res, p := mustConvert(t, "singbox.json", Options{})
	if res.Proxies != 6 {
		t.Errorf("want 6 proxies, got %d: %v", res.Proxies, res.Warnings)
	}
	sel := findGroup(p, "select")
	if sel["type"] != "select" || fmt.Sprint(sel["proxies"]) != "[auto ss-out vless-reality DIRECT]" {
		t.Errorf("selector wrong: %v", sel)
	}
	if a := findGroup(p, "auto"); a["type"] != "url-test" || a["interval"] != 300 {
		t.Errorf("urltest wrong: %v", a)
	}
	if hy := findProxy(p, "hy2"); hy["ports"] != "20000-30000" || hy["skip-cert-verify"] != true {
		t.Errorf("hy2 wrong: %v", hy)
	}
	if wg := findProxy(p, "wg-ep"); wg["server"] != "wg.example.com" || wg["ipv6"] != "fd00::2" {
		t.Errorf("wireguard endpoint wrong: %v", wg)
	}
	if !hasWarning(res, "route rules are not converted") {
		t.Error("expected route warning")
	}
	if p.Rules[len(p.Rules)-1] != "MATCH,select" {
		t.Errorf("rules should route via first group: %v", p.Rules)
	}
}

func TestClash(t *testing.T) {
	src := readTestdata(t, "clash_with_rules.yaml")
	res, err := Convert(src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.YAML != string(src) || res.Format != FormatClash || res.Rules != 1 {
		t.Error("clash with rules must pass through unchanged")
	}
	res2, p := mustConvert(t, "clash_no_rules.yaml", Options{})
	if res2.Proxies != 2 || findGroup(p, TemplateGroupProxy) == nil || !hasRule(p, "MATCH,Proxy") {
		t.Errorf("clash without rules should get template: %s", res2.YAML)
	}
}

func TestUserinfo(t *testing.T) {
	u := ParseUserinfo("upload=1234; download=5678;total=10737418240; expire=1767225600")
	if u.Upload != 1234 || u.Download != 5678 || u.Total != 10737418240 || u.Expire != 1767225600 {
		t.Errorf("userinfo wrong: %+v", u)
	}
	if ParseUserinfo("garbage").Total != 0 {
		t.Error("garbage should parse to zero")
	}
	if u := ParseUserinfo("upload=1.5e3; expire="); u.Upload != 1500 || u.Expire != 0 {
		t.Errorf("float/empty handling wrong: %+v", u)
	}
}

func TestSplitTop(t *testing.T) {
	got := splitTop(`AND,((DOMAIN,a.com),(DST-PORT,443)),Proxy`, ',')
	if len(got) != 3 || got[1] != "((DOMAIN,a.com),(DST-PORT,443))" {
		t.Errorf("splitTop logical: %q", got)
	}
	got = splitTop(`select, A, include-other-group="X, Y", B`, ',')
	if len(got) != 4 || got[2] != `include-other-group="X, Y"` {
		t.Errorf("splitTop quotes: %q", got)
	}
}
