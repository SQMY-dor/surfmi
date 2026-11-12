// SPDX-License-Identifier: GPL-3.0-or-later

package convert

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/dlclark/regexp2"
	"go.yaml.in/yaml/v3"
)

// OverrideMode controls how a rule template is merged into a converted
// subscription.
type OverrideMode string

const (
	// OverrideReplace drops the subscription's groups and rules and uses the
	// template's; only the subscription's proxies (and proxy-providers) are
	// kept. This is what subconverter's "external config" does.
	OverrideReplace OverrideMode = "replace"
	// OverrideAppend keeps the subscription's groups and rules, adds the
	// template's groups and inserts the template's rules just before the
	// subscription's final MATCH rule.
	OverrideAppend OverrideMode = "append"
	// OverridePrepend is like OverrideAppend but puts the template's rules
	// in front of the subscription's.
	OverridePrepend OverrideMode = "prepend"
)

// ParseOverrideMode maps a user/bridge string to a mode (default replace).
func ParseOverrideMode(s string) OverrideMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "append", "append-rules", "append_rules":
		return OverrideAppend
	case "prepend", "prepend-rules", "prepend_rules":
		return OverridePrepend
	}
	return OverrideReplace
}

// Override is a local rule template applied after conversion.
type Override struct {
	// Template is the template text: a Surge-style snippet with
	// [Proxy Group] / [Rule] sections, or Clash/mihomo YAML with
	// proxy-groups / rules / rule-providers.
	Template string
	Mode     OverrideMode
}

// Template is a parsed rule template.
type Template struct {
	Format        Format // FormatSurge or FormatClash
	Groups        []*OMap
	Rules         []string
	RuleProviders *OMap
	Warnings      []string
}

var templateSurgeRe = regexp.MustCompile(`(?mi)^\s*\[(Proxy Group|Rule)\]\s*$`)

// ParseTemplate parses template text in either supported syntax.
func ParseTemplate(text string) (*Template, error) {
	text = strings.TrimPrefix(text, "\ufeff")
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("template is empty")
	}
	if templateSurgeRe.MatchString(text) {
		return parseSurgeTemplate(text)
	}
	return parseClashTemplate(text)
}

func parseSurgeTemplate(text string) (*Template, error) {
	p := newProfile()
	p.TestURL = DefaultTestURL
	p.template = true
	// Only the group and rule sections matter; parse them with the regular
	// Surge parser so the syntax is identical to a full Surge profile.
	var keep strings.Builder
	cur := ""
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			cur = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
		}
		if cur == "proxy group" || cur == "rule" {
			keep.WriteString(raw)
			keep.WriteByte('\n')
		}
	}
	sp, err := parseSurgeInto(p, keep.String(), Options{})
	if err != nil {
		return nil, err
	}
	t := &Template{Format: FormatSurge, Groups: sp.Groups, Rules: sp.Rules, RuleProviders: sp.RuleProviders, Warnings: sp.warnings}
	if len(t.Groups) == 0 && len(t.Rules) == 0 {
		return nil, fmt.Errorf("template has no [Proxy Group] or [Rule] entries")
	}
	return t, nil
}

func parseClashTemplate(text string) (*Template, error) {
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(text), &raw); err != nil {
		return nil, fmt.Errorf("template is neither a Surge snippet nor valid YAML: %w", err)
	}
	if raw == nil {
		return nil, fmt.Errorf("template is empty")
	}
	t := &Template{Format: FormatClash, RuleProviders: NewOMap()}
	if list, ok := raw["proxy-groups"].([]any); ok {
		for i, x := range list {
			m, ok := x.(map[string]any)
			if !ok {
				t.Warnings = append(t.Warnings, fmt.Sprintf("template proxy-groups[%d] is not a map, skipped", i))
				continue
			}
			g := mapToOMap(m)
			if g.GetString("name") == "" {
				t.Warnings = append(t.Warnings, fmt.Sprintf("template proxy-groups[%d] has no name, skipped", i))
				continue
			}
			if g.GetString("type") == "" {
				g.Set("type", "select")
			}
			t.Groups = append(t.Groups, g)
		}
	}
	if prov, ok := raw["rule-providers"].(map[string]any); ok {
		for _, k := range sortedKeys(prov) {
			if m, ok := prov[k].(map[string]any); ok {
				t.RuleProviders.Set(k, mapToOMap(m))
			}
		}
	}
	if list, ok := raw["rules"].([]any); ok {
		for _, x := range list {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
				t.Rules = append(t.Rules, strings.TrimSpace(s))
			}
		}
	}
	if len(t.Groups) == 0 && len(t.Rules) == 0 {
		return nil, fmt.Errorf("template has no proxy-groups or rules")
	}
	return t, nil
}

// ApplyOverride merges template into profile p according to mode.
func (p *Profile) ApplyOverride(t *Template, mode OverrideMode) {
	for _, w := range t.Warnings {
		p.warn("template: %s", w)
	}
	srcGroups := p.Groups
	tmplGroups := cloneGroups(t.Groups)

	switch mode {
	case OverrideAppend, OverridePrepend:
		if len(p.Rules) == 0 {
			applyTemplate(p)
			srcGroups = p.Groups
		}
		// template groups replace same-named subscription groups
		byName := map[string]int{}
		merged := append([]*OMap(nil), srcGroups...)
		for i, g := range merged {
			byName[g.GetString("name")] = i
		}
		for _, g := range tmplGroups {
			if i, ok := byName[g.GetString("name")]; ok {
				p.warn("template group %q replaces the subscription's group of the same name", g.GetString("name"))
				merged[i] = g
				continue
			}
			merged = append(merged, g)
		}
		p.Groups = merged
		p.resolveGroups(tmplGroups)
		mergeRuleProviders(p, t.RuleProviders)
		rules := p.fixRules(t.Rules)
		p.Rules = mergeRules(p.Rules, rules, mode)
	default: // replace
		if len(tmplGroups) == 0 {
			// rules-only template: keep (or generate) the subscription's groups
			if len(p.Groups) == 0 {
				applyTemplate(p)
			}
		} else {
			p.Groups = tmplGroups
			p.resolveGroups(tmplGroups)
		}
		p.RuleProviders = NewOMap()
		mergeRuleProviders(p, t.RuleProviders)
		if len(t.Rules) == 0 {
			p.Rules = DefaultRules(p.firstGroupOr("DIRECT"))
		} else {
			p.Rules = p.fixRules(t.Rules)
		}
		if !hasMatch(p.Rules) {
			p.Rules = append(p.Rules, "MATCH,"+p.firstGroupOr("DIRECT"))
		}
	}
	p.pruneRuleProviders()
	p.dedupeWarnings()
}

func cloneGroups(in []*OMap) []*OMap {
	out := make([]*OMap, 0, len(in))
	for _, g := range in {
		c := NewOMap()
		for _, k := range g.Keys() {
			v, _ := g.Get(k)
			switch vv := v.(type) {
			case []string:
				v = append([]string(nil), vv...)
			case []any:
				v = append([]any(nil), vv...)
			}
			c.Set(k, v)
		}
		out = append(out, c)
	}
	return out
}

func (p *Profile) firstGroupOr(def string) string {
	if len(p.Groups) > 0 {
		return p.Groups[0].GetString("name")
	}
	return def
}

func hasMatch(rules []string) bool {
	return len(rules) > 0 && strings.HasPrefix(strings.ToUpper(rules[len(rules)-1]), "MATCH,")
}

// mergeRules inserts add into base. A MATCH in add is only kept when base
// has none.
func mergeRules(base, add []string, mode OverrideMode) []string {
	baseMatch := hasMatch(base)
	if baseMatch && hasMatch(add) {
		add = add[:len(add)-1]
	}
	if mode == OverridePrepend {
		return append(append([]string(nil), add...), base...)
	}
	if !baseMatch {
		return append(append([]string(nil), base...), add...)
	}
	out := append([]string(nil), base[:len(base)-1]...)
	out = append(out, add...)
	return append(out, base[len(base)-1])
}

func mergeRuleProviders(p *Profile, rp *OMap) {
	if rp == nil {
		return
	}
	for _, k := range rp.Keys() {
		v, _ := rp.Get(k)
		if _, exists := p.RuleProviders.Get(k); exists {
			p.warn("template rule-provider %q replaces the subscription's provider of the same name", k)
		}
		p.RuleProviders.Set(k, v)
	}
}

// pruneRuleProviders drops rule-providers no rule refers to.
func (p *Profile) pruneRuleProviders() {
	used := map[string]bool{}
	for _, r := range p.Rules {
		for _, name := range ruleSetRefs(r) {
			used[name] = true
		}
	}
	for _, k := range p.RuleProviders.Keys() {
		if !used[k] {
			p.RuleProviders.Delete(k)
		}
	}
}

var ruleSetRefRe = regexp.MustCompile(`(?i)(?:^|\()RULE-SET,([^,()]+)`)

func ruleSetRefs(rule string) []string {
	var out []string
	for _, m := range ruleSetRefRe.FindAllStringSubmatch(rule, -1) {
		out = append(out, strings.TrimSpace(m[1]))
	}
	return out
}

// ------------------------------------------------------------ group resolve

// realProxyNames lists subscription proxies that a template may pull in
// (built-in direct/reject style entries are skipped).
func (p *Profile) realProxyNames() (names []string, types map[string]string) {
	types = map[string]string{}
	for _, px := range p.Proxies {
		t := strings.ToLower(px.GetString("type"))
		switch t {
		case "direct", "reject", "dns":
			continue
		}
		n := px.GetString("name")
		names = append(names, n)
		types[n] = t
	}
	return
}

// compileFilters compiles a mihomo-style filter: several patterns may be
// joined with a backtick. Patterns use .NET/ICU-like syntax (lookahead etc.)
// like mihomo and Surge do.
func compileFilters(expr string) ([]*regexp2.Regexp, error) {
	var out []*regexp2.Regexp
	for _, part := range strings.Split(expr, "`") {
		if part == "" {
			continue
		}
		re, err := regexp2.Compile(part, regexp2.None)
		if err != nil {
			return nil, err
		}
		re.MatchTimeout = 200 * time.Millisecond
		out = append(out, re)
	}
	return out, nil
}

func anyMatch(res []*regexp2.Regexp, s string) bool {
	for _, re := range res {
		if ok, _ := re.MatchString(s); ok {
			return true
		}
	}
	return false
}

func toStrings(v any) []string {
	switch vv := v.(type) {
	case []string:
		return vv
	case []any:
		out := make([]string, 0, len(vv))
		for _, x := range vv {
			if s, ok := x.(string); ok {
				out = append(out, s)
			} else if x != nil {
				out = append(out, fmt.Sprint(x))
			}
		}
		return out
	case string:
		if vv != "" {
			return []string{vv}
		}
	}
	return nil
}

func boolVal(v any, ok bool) bool {
	if !ok {
		return false
	}
	switch vv := v.(type) {
	case bool:
		return vv
	case string:
		return parseBool(vv)
	}
	return false
}

// resolveGroups expands include-all / filter of the template groups into
// explicit proxy lists, removes references to unknown names and drops
// groups that end up empty. p.Groups must already contain tmplGroups.
func (p *Profile) resolveGroups(tmplGroups []*OMap) {
	isTmpl := map[*OMap]bool{}
	for _, g := range tmplGroups {
		isTmpl[g] = true
	}
	all, types := p.realProxyNames()
	proxySet := map[string]bool{}
	for _, px := range p.Proxies {
		proxySet[px.GetString("name")] = true
	}
	providers := p.Providers.Keys()
	providerSet := map[string]bool{}
	for _, k := range providers {
		providerSet[k] = true
	}

	for _, g := range p.Groups {
		if !isTmpl[g] {
			continue
		}
		name := g.GetString("name")
		incAll := boolVal(g.Get("include-all"))
		incProxies := incAll || boolVal(g.Get("include-all-proxies"))
		incProviders := incAll || boolVal(g.Get("include-all-providers"))
		filterExpr, _ := g.Get("filter")
		excludeExpr, _ := g.Get("exclude-filter")
		excludeType, _ := g.Get("exclude-type")
		fs, _ := filterExpr.(string)
		es, _ := excludeExpr.(string)
		ets, _ := excludeType.(string)

		filters, err := compileFilters(fs)
		if err != nil {
			p.warn("template group %q: invalid filter %q (%v), ignored", name, fs, err)
			filters = nil
		}
		excludes, err := compileFilters(es)
		if err != nil {
			p.warn("template group %q: invalid exclude-filter %q (%v), ignored", name, es, err)
			excludes = nil
		}
		exTypes := map[string]bool{}
		for _, t := range strings.Split(ets, "|") {
			if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
				exTypes[t] = true
			}
		}

		members := toStrings(firstOf(g, "proxies"))
		if incProxies {
			for _, n := range filterNames(all, filters, excludes, exTypes, types) {
				members = append(members, n)
			}
		}
		g.Set("proxies", uniqueStrings(members))

		var use []string
		for _, u := range toStrings(firstOf(g, "use")) {
			if providerSet[u] {
				use = append(use, u)
			} else {
				p.warn("template group %q: proxy-provider %q not in subscription, removed", name, u)
			}
		}
		if incProviders {
			use = append(use, providers...)
		}
		use = uniqueStrings(use)
		g.Delete("use")
		if len(use) > 0 {
			g.Set("use", use)
		} else {
			// filters were applied here; providers are the only thing mihomo
			// would still filter at runtime
			g.Delete("filter")
			g.Delete("exclude-filter")
			g.Delete("exclude-type")
		}
		g.Delete("include-all")
		g.Delete("include-all-proxies")
		g.Delete("include-all-providers")

		switch g.GetString("type") {
		case "url-test", "fallback", "load-balance":
			if g.GetString("url") == "" {
				g.Set("url", orDefault(p.TestURL, DefaultTestURL))
			}
			if _, ok := g.Get("interval"); !ok {
				g.Set("interval", 300)
			}
		}
	}

	// iteratively drop unknown members and empty template groups
	dropped := map[string]bool{}
	for changed := true; changed; {
		changed = false
		groupSet := map[string]bool{}
		for _, g := range p.Groups {
			groupSet[g.GetString("name")] = true
		}
		var keep []*OMap
		for _, g := range p.Groups {
			name := g.GetString("name")
			members := toStrings(firstOf(g, "proxies"))
			var ok []string
			for _, m := range members {
				switch {
				case m == name:
					p.warn("group %q: refers to itself, removed", name)
				case groupSet[m] || proxySet[m] || isBuiltinMember(m):
					ok = append(ok, m)
				case dropped[m]:
					// already reported when the group was dropped
				default:
					if isTmpl[g] {
						p.warn("template group %q: member %q not found, removed", name, m)
					} else {
						p.warn("group %q: member %q not found, removed", name, m)
					}
				}
			}
			if len(ok) != len(members) {
				changed = true
			}
			g.Delete("proxies")
			if len(ok) > 0 {
				g.Set("proxies", ok)
			}
			_, hasUse := g.Get("use")
			if !isTmpl[g] {
				// subscription groups (append mode) are kept as they were;
				// mihomo resolves their own include-all at runtime
				if len(ok) == 0 && !hasUse && !boolVal(g.Get("include-all")) &&
					!boolVal(g.Get("include-all-proxies")) && !boolVal(g.Get("include-all-providers")) {
					g.Set("proxies", []string{"DIRECT"})
				}
				keep = append(keep, g)
				continue
			}
			if len(ok) == 0 && !hasUse {
				p.warn("group %q matched no proxies and was skipped", name)
				dropped[name] = true
				changed = true
				continue
			}
			keep = append(keep, g)
		}
		p.Groups = keep
	}
	// the proxies key comes before use/url etc. in the output for readability
	for _, g := range p.Groups {
		reorderGroup(g)
	}
	p.droppedGroups = dropped
}

func firstOf(g *OMap, key string) any {
	v, _ := g.Get(key)
	return v
}

func filterNames(all []string, filters, excludes []*regexp2.Regexp, exTypes map[string]bool, types map[string]string) []string {
	var picked []string
	if len(filters) == 0 {
		picked = append(picked, all...)
	} else {
		// like mihomo: each filter pattern adds its matches in order
		seen := map[string]bool{}
		for _, re := range filters {
			for _, n := range all {
				if seen[n] {
					continue
				}
				if ok, _ := re.MatchString(n); ok {
					seen[n] = true
					picked = append(picked, n)
				}
			}
		}
	}
	var out []string
	for _, n := range picked {
		if anyMatch(excludes, n) || exTypes[types[n]] {
			continue
		}
		out = append(out, n)
	}
	return out
}

func isBuiltinMember(n string) bool {
	switch strings.ToUpper(n) {
	case "DIRECT", "REJECT", "REJECT-DROP", "PASS", "COMPATIBLE":
		return true
	}
	return false
}

var groupKeyOrder = []string{"name", "type", "proxies", "use"}

func reorderGroup(g *OMap) {
	var keys []string
	for _, k := range groupKeyOrder {
		if _, ok := g.Get(k); ok {
			keys = append(keys, k)
		}
	}
	for _, k := range g.keys {
		found := false
		for _, o := range groupKeyOrder {
			if k == o {
				found = true
				break
			}
		}
		if !found {
			keys = append(keys, k)
		}
	}
	g.keys = keys
}

// ------------------------------------------------------------ rule fixing

// rulePolicyIndex returns the index of the policy token in a mihomo rule.
func rulePolicyIndex(tokens []string) int {
	if len(tokens) < 2 {
		return -1
	}
	if strings.EqualFold(tokens[0], "MATCH") {
		return 1
	}
	i := len(tokens) - 1
	for i > 1 {
		switch strings.ToLower(tokens[i]) {
		case "no-resolve", "src":
			i--
			continue
		}
		break
	}
	if i < 2 {
		return -1
	}
	return i
}

// fixRules validates template rules against the final groups: rules whose
// policy was a skipped group are routed to the first remaining group (or
// DIRECT), and RULE-SET rules naming an unknown provider are dropped.
func (p *Profile) fixRules(rules []string) []string {
	targets := map[string]bool{}
	for _, g := range p.Groups {
		targets[g.GetString("name")] = true
	}
	for _, px := range p.Proxies {
		targets[px.GetString("name")] = true
	}
	fallback := p.firstGroupOr("DIRECT")
	var out []string
	for _, r := range rules {
		tokens := splitTop(r, ',')
		idx := rulePolicyIndex(tokens)
		if idx < 0 {
			p.warn("template rule %q is malformed, skipped", truncate(r, 80))
			continue
		}
		missingSet := false
		for _, ref := range ruleSetRefs(r) {
			if _, ok := p.RuleProviders.Get(ref); !ok {
				missingSet = true
				p.warn("template rule %q refers to unknown rule-provider %q, skipped", truncate(r, 80), ref)
			}
		}
		if missingSet {
			continue
		}
		pol := strings.TrimSpace(tokens[idx])
		if !targets[pol] && !isBuiltinMember(pol) {
			if p.droppedGroups[pol] {
				p.warn("rules for skipped group %q now use %q", pol, fallback)
			} else {
				p.warn("template rule %q: policy %q not found, using %q", truncate(r, 80), pol, fallback)
			}
			tokens[idx] = fallback
			r = strings.Join(tokens, ",")
		}
		out = append(out, r)
	}
	return out
}

// dedupeWarnings collapses repeated identical warnings (many rules can point
// at the same skipped group).
func (p *Profile) dedupeWarnings() {
	seen := map[string]bool{}
	out := p.warnings[:0]
	for _, w := range p.warnings {
		if seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	p.warnings = out
}
