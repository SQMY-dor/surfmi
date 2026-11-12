// SPDX-License-Identifier: GPL-3.0-or-later

package convert

import (
	"reflect"
	"strings"
	"testing"

	"github.com/metacubex/mihomo/config"
	C "github.com/metacubex/mihomo/constant"
)

func overrideOpt(t *testing.T, tmplFile string, mode OverrideMode) Options {
	t.Helper()
	return Options{Override: &Override{Template: string(readTestdata(t, tmplFile)), Mode: mode}}
}

func groupByName(p parsed, name string) map[string]any {
	for _, g := range p.ProxyGroups {
		if g["name"] == name {
			return g
		}
	}
	return nil
}

func groupMembers(p parsed, name string) []string {
	g := groupByName(p, name)
	if g == nil {
		return nil
	}
	var out []string
	if list, ok := g["proxies"].([]any); ok {
		for _, x := range list {
			out = append(out, x.(string))
		}
	}
	return out
}

func TestOverrideReplaceClash(t *testing.T) {
	res, p := mustConvert(t, "template_sub.yaml", overrideOpt(t, "template_custom.yaml", OverrideReplace))

	if len(p.Proxies) != 6 {
		t.Errorf("proxies = %d, want all 6 subscription proxies kept", len(p.Proxies))
	}
	if groupByName(p, "SubSelect") != nil {
		t.Error("subscription group should be replaced")
	}
	if got, want := groupMembers(p, "香港"), []string{"🇭🇰 香港 01", "HK02 IEPL"}; !reflect.DeepEqual(got, want) {
		t.Errorf("香港 = %v, want %v", got, want)
	}
	if groupByName(p, "新加坡") != nil || groupByName(p, "流媒体") != nil {
		t.Error("empty groups 新加坡 / 流媒体 should be skipped")
	}
	if got, want := groupMembers(p, "非港"), []string{"🇯🇵 日本 东京", "🇺🇸 美国 洛杉矶"}; !reflect.DeepEqual(got, want) {
		t.Errorf("非港 = %v, want %v (vmess excluded by exclude-type)", got, want)
	}
	sel := groupMembers(p, "节点选择")
	want := []string{"香港", "DIRECT", "🇭🇰 香港 01", "HK02 IEPL", "🇯🇵 日本 东京", "🇺🇸 美国 洛杉矶", "Russia Moscow"}
	if !reflect.DeepEqual(sel, want) {
		t.Errorf("节点选择 = %v, want %v", sel, want)
	}
	for _, g := range p.ProxyGroups {
		for _, k := range []string{"include-all", "include-all-proxies", "filter", "exclude-filter", "exclude-type"} {
			if _, ok := g[k]; ok {
				t.Errorf("group %v still has %s after resolution", g["name"], k)
			}
		}
	}
	h := groupByName(p, "香港")
	if h["url"] != DefaultTestURL || h["interval"] != 300 {
		t.Errorf("url-test defaults missing: %v", h)
	}

	wantRules := []string{
		"RULE-SET,cn,DIRECT",
		"DOMAIN-SUFFIX,netflix.com,节点选择",
		"DOMAIN-SUFFIX,missing.example,节点选择",
		"MATCH,节点选择",
	}
	if !reflect.DeepEqual(p.Rules, wantRules) {
		t.Errorf("rules = %v\nwant %v", p.Rules, wantRules)
	}
	if _, ok := p.RuleProviders["cn"]; !ok || len(p.RuleProviders) != 1 {
		t.Errorf("rule-providers = %v, want only template's cn", p.RuleProviders)
	}
	for _, w := range []string{"新加坡", "NoSuchGroup", `"nope"`} {
		if !hasWarning(res, w) {
			t.Errorf("missing warning about %s: %v", w, res.Warnings)
		}
	}
}

func TestOverrideReplaceSurgeTemplate(t *testing.T) {
	// Clash subscription + Surge-syntax template
	_, p := mustConvert(t, "template_sub.yaml", overrideOpt(t, "template_surge.conf", OverrideReplace))
	if got, want := groupMembers(p, "HK"), []string{"🇭🇰 香港 01", "HK02 IEPL"}; !reflect.DeepEqual(got, want) {
		t.Errorf("HK = %v, want %v", got, want)
	}
	if got := groupMembers(p, "JP"); !reflect.DeepEqual(got, []string{"🇯🇵 日本 东京"}) {
		t.Errorf("JP = %v", got)
	}
	if groupByName(p, "SG") != nil {
		t.Error("SG matched nothing and should be skipped")
	}
	px := groupMembers(p, "Proxy")
	if len(px) != 3+6 || px[0] != "HK" || px[1] != "JP" || px[2] != "DIRECT" {
		t.Errorf("Proxy = %v", px)
	}
	if !contains(p.Rules, "DOMAIN-SUFFIX,sg-only.example,Proxy") {
		t.Errorf("rule for skipped SG should fall back to first group: %v", p.Rules)
	}
	if !contains(p.Rules, "GEOSITE,cn,DIRECT") || p.Rules[len(p.Rules)-1] != "MATCH,Proxy" {
		t.Errorf("rules = %v", p.Rules)
	}
	var mrs map[string]any
	for _, rp := range p.RuleProviders {
		if strings.HasSuffix(rp["url"].(string), "geoip/cn.mrs") {
			mrs = rp
		}
	}
	if mrs == nil || mrs["format"] != "mrs" || mrs["behavior"] != "ipcidr" {
		t.Errorf("mrs rule-set provider = %v", mrs)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestOverrideAppendAndPrepend(t *testing.T) {
	_, p := mustConvert(t, "template_sub.yaml", overrideOpt(t, "template_append.yaml", OverrideAppend))
	want := []string{
		"RULE-SET,sub_rules,SubSelect",
		"DOMAIN-SUFFIX,example.org,SubSelect",
		"DOMAIN-SUFFIX,company.example,DIRECT",
		"DOMAIN-KEYWORD,tracker,REJECT",
		"MATCH,SubSelect",
	}
	if !reflect.DeepEqual(p.Rules, want) {
		t.Errorf("append rules = %v\nwant %v", p.Rules, want)
	}
	if groupByName(p, "SubSelect") == nil {
		t.Error("append mode must keep subscription groups")
	}
	if _, ok := p.RuleProviders["sub_rules"]; !ok {
		t.Error("append mode must keep subscription rule-providers")
	}

	_, p = mustConvert(t, "template_sub.yaml", overrideOpt(t, "template_append.yaml", OverridePrepend))
	want = []string{
		"DOMAIN-SUFFIX,company.example,DIRECT",
		"DOMAIN-KEYWORD,tracker,REJECT",
		"RULE-SET,sub_rules,SubSelect",
		"DOMAIN-SUFFIX,example.org,SubSelect",
		"MATCH,SubSelect",
	}
	if !reflect.DeepEqual(p.Rules, want) {
		t.Errorf("prepend rules = %v\nwant %v", p.Rules, want)
	}
}

func TestOverrideAppendWithoutSourceRules(t *testing.T) {
	// URI list has no rules: defaults are generated, then template rules go before MATCH
	_, p := mustConvert(t, "uri_plain.txt", overrideOpt(t, "template_append.yaml", OverrideAppend))
	n := len(p.Rules)
	if n < 3 || p.Rules[n-1] != "MATCH,"+TemplateGroupProxy || p.Rules[n-2] != "DOMAIN-KEYWORD,tracker,REJECT" {
		t.Errorf("rules = %v", p.Rules)
	}
}

func TestOverrideRulesOnlyReplaceKeepsGroups(t *testing.T) {
	_, p := mustConvert(t, "template_sub.yaml", overrideOpt(t, "template_append.yaml", OverrideReplace))
	if groupByName(p, "SubSelect") == nil {
		t.Error("rules-only template in replace mode keeps subscription groups")
	}
	if p.Rules[len(p.Rules)-1] != "MATCH,DIRECT" || len(p.Rules) != 3 {
		t.Errorf("rules = %v", p.Rules)
	}
	if len(p.RuleProviders) != 0 {
		t.Errorf("unused providers should be pruned: %v", p.RuleProviders)
	}
}

func TestOverrideNoTemplateKeepsSubscription(t *testing.T) {
	res, err := Convert(readTestdata(t, "template_sub.yaml"), Options{Override: &Override{Template: "  "}})
	if err != nil {
		t.Fatal(err)
	}
	if res.YAML != string(readTestdata(t, "template_sub.yaml")) {
		t.Error("empty template must leave a Clash profile with rules untouched")
	}
}

func TestOverrideInvalidTemplate(t *testing.T) {
	for _, tmpl := range []string{"just some text", "foo: bar\n", "[Rule]\n"} {
		_, err := Convert(readTestdata(t, "template_sub.yaml"), Options{Override: &Override{Template: tmpl}})
		if err == nil || !strings.Contains(err.Error(), "rule template") {
			t.Errorf("template %q: err = %v, want rule template error", tmpl, err)
		}
	}
}

func TestParseOverrideMode(t *testing.T) {
	for in, want := range map[string]OverrideMode{"": OverrideReplace, "replace": OverrideReplace, "APPEND": OverrideAppend, "prepend": OverridePrepend, "junk": OverrideReplace} {
		if got := ParseOverrideMode(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}

func TestBuiltinTemplates(t *testing.T) {
	list := BuiltinTemplates()
	if len(list) != 3 {
		t.Fatalf("built-in templates = %d", len(list))
	}
	for _, bt := range list {
		if _, err := ParseTemplate(bt.Content); err != nil {
			t.Errorf("%s: %v", bt.ID, err)
		}
	}
	_, p := mustConvert(t, "template_sub.yaml", Options{Override: &Override{Template: mustBuiltin(t, "builtin:split")}})
	if got, want := groupMembers(p, "香港"), []string{"🇭🇰 香港 01", "HK02 IEPL"}; !reflect.DeepEqual(got, want) {
		t.Errorf("split 香港 = %v, want %v", got, want)
	}
	if groupByName(p, "台湾") != nil || groupByName(p, "新加坡") != nil {
		t.Error("split: empty region groups must be skipped")
	}
	if got := groupMembers(p, "AI"); !reflect.DeepEqual(got, []string{"美国", "日本", "节点选择"}) {
		t.Errorf("split AI = %v", got)
	}
	auto := groupMembers(p, "自动选择")
	if contains(auto, "剩余流量：100GB") || len(auto) != 5 {
		t.Errorf("info node must be excluded from 自动选择: %v", auto)
	}
	if len(p.RuleProviders) < 15 {
		t.Errorf("split rule-providers = %d", len(p.RuleProviders))
	}
	for name, rp := range p.RuleProviders {
		if rp["format"] != "mrs" || !strings.HasPrefix(rp["url"].(string), "https://") {
			t.Errorf("provider %s = %v", name, rp)
		}
	}
}

func mustBuiltin(t *testing.T, id string) string {
	t.Helper()
	c, ok := BuiltinTemplateContent(id)
	if !ok {
		t.Fatalf("no built-in %s", id)
	}
	return c
}

// TestMihomoAcceptsTemplateOutput validates overridden configs with mihomo.
func TestMihomoAcceptsTemplateOutput(t *testing.T) {
	C.SetHomeDir(t.TempDir())
	cases := map[string]Options{
		"custom":  overrideOpt(t, "template_custom.yaml", OverrideReplace),
		"surge":   overrideOpt(t, "template_surge.conf", OverrideReplace),
		"append":  overrideOpt(t, "template_append.yaml", OverrideAppend),
		"simple":  {Override: &Override{Template: mustBuiltin(t, "builtin:simple")}},
		"split":   {Override: &Override{Template: mustBuiltin(t, "builtin:split")}},
		"example": {Override: &Override{Template: mustBuiltin(t, "builtin:surge-example")}},
	}
	subs := []string{"template_sub.yaml", "surge_full.conf", "uri_plain.txt"}
	for name, opt := range cases {
		for _, sub := range subs {
			t.Run(name+"/"+sub, func(t *testing.T) {
				opt.Fetcher = testFetcher
				res, err := Convert(readTestdata(t, sub), opt)
				if err != nil {
					t.Fatal(err)
				}
				raw, err := config.UnmarshalRawConfig([]byte(stripGeoRules(res.YAML)))
				if err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if _, err := config.ParseRawConfig(raw); err != nil {
					t.Fatalf("mihomo rejected: %v\n%s", err, res.YAML)
				}
			})
		}
	}
}
