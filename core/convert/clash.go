// SPDX-License-Identifier: GPL-3.0-or-later

package convert

import (
	"fmt"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// convertClash passes Clash/mihomo YAML through untouched when it already
// carries rules; otherwise its proxies/providers/groups are kept and the
// default rule template is added.
func convertClash(data []byte, opt Options) (*Result, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("clash yaml: %w", err)
	}
	rules, _ := raw["rules"].([]any)
	proxies, _ := raw["proxies"].([]any)
	groups, _ := raw["proxy-groups"].([]any)
	hasOverride := opt.Override != nil && strings.TrimSpace(opt.Override.Template) != ""
	if len(rules) > 0 && !opt.ForceTemplate && !hasOverride {
		return &Result{
			Format:   FormatClash,
			YAML:     string(data),
			Warnings: []string{},
			Proxies:  len(proxies),
			Groups:   len(groups),
			Rules:    len(rules),
		}, nil
	}
	p, err := clashToProfile(data)
	if err != nil {
		return nil, err
	}
	if opt.TestURL != "" {
		p.TestURL = opt.TestURL
	}
	if len(p.Proxies) == 0 && p.Providers.Len() == 0 {
		return nil, fmt.Errorf("no usable proxies found (clash)")
	}
	if err := p.finishRules(opt); err != nil {
		return nil, err
	}
	out, err := p.Render()
	if err != nil {
		return nil, err
	}
	return &Result{
		Format:   FormatClash,
		YAML:     out,
		Warnings: nonNil(p.warnings),
		Proxies:  len(p.Proxies),
		Groups:   len(p.Groups),
		Rules:    len(p.Rules),
	}, nil
}

// clashToProfile extracts proxies, providers and groups from Clash YAML.
func clashToProfile(data []byte) (*Profile, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("clash yaml: %w", err)
	}
	p := newProfile()
	p.TestURL = DefaultTestURL
	if list, ok := raw["proxies"].([]any); ok {
		for _, x := range list {
			if m, ok := x.(map[string]any); ok {
				p.addProxy(mapToOMap(m))
			}
		}
	}
	if prov, ok := raw["proxy-providers"].(map[string]any); ok {
		for _, k := range sortedKeys(prov) {
			if m, ok := prov[k].(map[string]any); ok {
				p.Providers.Set(k, mapToOMap(m))
			}
		}
	}
	if list, ok := raw["proxy-groups"].([]any); ok {
		for _, x := range list {
			if m, ok := x.(map[string]any); ok {
				p.Groups = append(p.Groups, mapToOMap(m))
			}
		}
	}
	if d, ok := raw["dns"].(map[string]any); ok {
		p.DNS = mapToOMap(d)
	}
	if h, ok := raw["hosts"].(map[string]any); ok {
		for _, k := range sortedKeys(h) {
			p.Hosts.Set(k, h[k])
		}
	}
	if prov, ok := raw["rule-providers"].(map[string]any); ok {
		for _, k := range sortedKeys(prov) {
			if m, ok := prov[k].(map[string]any); ok {
				p.RuleProviders.Set(k, mapToOMap(m))
			}
		}
	}
	if list, ok := raw["rules"].([]any); ok {
		for _, x := range list {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
				p.Rules = append(p.Rules, strings.TrimSpace(s))
			}
		}
	}
	for _, k := range clashKeptTopKeys {
		if v, ok := raw[k]; ok {
			p.Top.Set(k, v)
		}
	}
	return p, nil
}

// clashKeptTopKeys are behaviour settings carried over from a Clash source
// when it is re-rendered (ports and controllers stay app-managed).
var clashKeptTopKeys = []string{
	"ipv6", "log-level", "unified-delay", "tcp-concurrent", "find-process-mode",
	"global-client-fingerprint", "sniffer", "geodata-mode", "geox-url",
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

var preferredOrder = []string{"name", "type", "server", "port"}

func mapToOMap(m map[string]any) *OMap {
	o := NewOMap()
	for _, k := range preferredOrder {
		if v, ok := m[k]; ok {
			o.Set(k, v)
		}
	}
	for _, k := range sortedKeys(m) {
		if _, done := o.Get(k); done {
			continue
		}
		o.Set(k, m[k])
	}
	return o
}

// TemplateGroupProxy / TemplateGroupAuto are the names of generated groups.
const (
	TemplateGroupProxy = "Proxy"
	TemplateGroupAuto  = "Auto"
)

// applyTemplate adds the default "China direct, everything else proxied"
// policy when the source has no rules of its own.
func applyTemplate(p *Profile) {
	names := p.proxyNames()
	var realProxies []string
	for i, n := range names {
		switch strings.ToLower(p.Proxies[i].GetString("type")) {
		case "direct", "reject", "dns":
			continue
		}
		realProxies = append(realProxies, n)
	}
	providers := p.Providers.Keys()

	mainGroup := ""
	if len(p.Groups) > 0 {
		// keep the source's groups, route through the first one
		mainGroup = p.Groups[0].GetString("name")
	} else {
		hasAny := len(realProxies) > 0 || len(providers) > 0
		sel := NewOMap().Set("name", TemplateGroupProxy).Set("type", "select")
		members := []string{}
		if hasAny {
			members = append(members, TemplateGroupAuto)
		}
		members = append(members, realProxies...)
		members = append(members, "DIRECT")
		sel.Set("proxies", members)
		if len(providers) > 0 {
			sel.Set("use", providers)
		}
		p.Groups = append(p.Groups, sel)
		if hasAny {
			auto := NewOMap().Set("name", TemplateGroupAuto).Set("type", "url-test")
			if len(realProxies) > 0 {
				auto.Set("proxies", realProxies)
			}
			if len(providers) > 0 {
				auto.Set("use", providers)
			}
			auto.Set("url", p.TestURL).Set("interval", 300).Set("tolerance", 50).Set("lazy", true)
			p.Groups = append(p.Groups, auto)
		}
		mainGroup = TemplateGroupProxy
	}
	p.Rules = DefaultRules(mainGroup)
}

// DefaultRules is the built-in rule template: private networks and
// mainland-China destinations go direct, everything else uses policy.
func DefaultRules(policy string) []string {
	rules := []string{
		"DOMAIN-SUFFIX,local,DIRECT",
		"DOMAIN-SUFFIX,lan,DIRECT",
	}
	for _, c := range lanCIDRs4 {
		rules = append(rules, "IP-CIDR,"+c+",DIRECT,no-resolve")
	}
	for _, c := range lanCIDRs6 {
		rules = append(rules, "IP-CIDR6,"+c+",DIRECT,no-resolve")
	}
	rules = append(rules,
		"DOMAIN-SUFFIX,cn,DIRECT",
		"GEOSITE,cn,DIRECT",
		"GEOIP,CN,DIRECT",
		"MATCH,"+policy,
	)
	return rules
}
