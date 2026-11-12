// SPDX-License-Identifier: GPL-3.0-or-later

// Package convert turns subscription/profile content in several formats
// (Surge .conf, Clash/mihomo YAML, base64 or plain URI lists, sing-box JSON)
// into a mihomo configuration. It is an independent implementation based on
// the publicly documented formats; it contains no code from Surge.
package convert

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Format identifies the detected source format.
type Format string

const (
	FormatSurge     Format = "surge"
	FormatSurgeList Format = "surge-list"
	FormatClash     Format = "clash"
	FormatURIList   Format = "uri-list"
	FormatSingBox   Format = "sing-box"
	FormatUnknown   Format = "unknown"
)

// DefaultTestURL is used for url-test/fallback groups when the source names none.
const DefaultTestURL = "http://www.gstatic.com/generate_204"

// Fetcher downloads remote resources referenced by a profile (Surge
// policy-path). It may be nil, in which case such resources are turned into
// mihomo proxy-providers that are fetched at runtime instead.
type Fetcher interface {
	Fetch(url string) ([]byte, error)
}

// FetcherFunc adapts a function to Fetcher.
type FetcherFunc func(url string) ([]byte, error)

func (f FetcherFunc) Fetch(url string) ([]byte, error) { return f(url) }

// Options control a conversion.
type Options struct {
	Fetcher Fetcher
	// TestURL overrides the latency test URL for generated groups.
	TestURL string
	// ForceTemplate replaces the source rules with the default template.
	ForceTemplate bool
	// Override applies a local rule template (groups + rules) after
	// conversion, keeping only the subscription's proxies when its mode is
	// OverrideReplace. Nil keeps the subscription's own groups and rules.
	Override *Override
}

// Managed carries Surge #!MANAGED-CONFIG metadata.
type Managed struct {
	URL      string `json:"url"`
	Interval int    `json:"interval"`
	Strict   bool   `json:"strict"`
}

// Result is the output of Convert.
type Result struct {
	Format   Format   `json:"format"`
	YAML     string   `json:"yaml"`
	Warnings []string `json:"warnings"`
	Proxies  int      `json:"proxies"`
	Groups   int      `json:"groups"`
	Rules    int      `json:"rules"`
	Managed  *Managed `json:"managed,omitempty"`
}

// JSON returns the result encoded as JSON (used by the Android bridge).
func (r *Result) JSON() string {
	b, _ := json.Marshal(r)
	return string(b)
}

// Profile is the format-neutral intermediate representation.
type Profile struct {
	Top           *OMap // misc top-level keys (ipv6, log-level, allow-lan, ...)
	DNS           *OMap
	Hosts         *OMap
	Proxies       []*OMap
	Providers     *OMap
	Groups        []*OMap
	RuleProviders *OMap
	Rules         []string
	TestURL       string
	Managed       *Managed

	warnings []string
	names    map[string]bool

	template      bool            // parsing a rule template, not a profile
	droppedGroups map[string]bool // template groups skipped as empty
}

func newProfile() *Profile {
	return &Profile{
		Top:           NewOMap(),
		Hosts:         NewOMap(),
		Providers:     NewOMap(),
		RuleProviders: NewOMap(),
		names:         map[string]bool{},
	}
}

func (p *Profile) warn(format string, a ...any) {
	p.warnings = append(p.warnings, fmt.Sprintf(format, a...))
}

// addProxy appends a proxy, renaming it if the name is already taken.
// It returns the final name.
func (p *Profile) addProxy(px *OMap) string {
	name := px.GetString("name")
	if name == "" {
		name = fmt.Sprintf("%s-%v", px.GetString("type"), px.vals["server"])
	}
	final := name
	for i := 2; p.names[final] || isBuiltinPolicy(final); i++ {
		final = fmt.Sprintf("%s %d", name, i)
	}
	px.Set("name", final)
	p.names[final] = true
	p.Proxies = append(p.Proxies, px)
	return final
}

func (p *Profile) proxyNames() []string {
	out := make([]string, 0, len(p.Proxies))
	for _, px := range p.Proxies {
		out = append(out, px.GetString("name"))
	}
	return out
}

func isBuiltinPolicy(n string) bool {
	switch strings.ToUpper(n) {
	case "DIRECT", "REJECT", "REJECT-DROP", "PASS", "COMPATIBLE", "GLOBAL":
		return true
	}
	return false
}

var (
	surgeSectionRe = regexp.MustCompile(`(?mi)^\s*\[(General|Proxy|Proxy Group|Rule|Host|MITM|Script|URL Rewrite)\]\s*$`)
	surgeLineRe    = regexp.MustCompile(`(?i)^[^=,\[\]]+=\s*(ss|vmess|trojan|snell|http|https|socks5|socks5-tls|tuic|tuic-v5|hysteria2|wireguard|anytls|ssh|direct|reject|custom)\s*(,|$)`)
)

// Detect guesses the format of data.
func Detect(data []byte) Format {
	s := strings.TrimSpace(strings.TrimPrefix(string(data), "\ufeff"))
	if s == "" {
		return FormatUnknown
	}
	if strings.HasPrefix(s, "{") {
		var probe map[string]json.RawMessage
		if json.Unmarshal([]byte(s), &probe) == nil {
			if _, ok := probe["outbounds"]; ok {
				return FormatSingBox
			}
			if _, ok := probe["endpoints"]; ok {
				return FormatSingBox
			}
		}
	}
	if surgeSectionRe.MatchString(s) {
		return FormatSurge
	}
	if looksLikeSurgeList(s) {
		return FormatSurgeList
	}
	var y map[string]any
	if yaml.Unmarshal([]byte(s), &y) == nil && y != nil {
		for _, k := range []string{"proxies", "proxy-providers", "proxy-groups"} {
			if _, ok := y[k]; ok {
				return FormatClash
			}
		}
	}
	if dec, ok := tryBase64(s); ok && strings.Contains(dec, "://") {
		return FormatURIList
	}
	if strings.Contains(s, "://") {
		return FormatURIList
	}
	return FormatUnknown
}

func looksLikeSurgeList(s string) bool {
	total, hit := 0, 0
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || isComment(line) {
			continue
		}
		total++
		if surgeLineRe.MatchString(line) {
			hit++
		}
	}
	return total > 0 && hit*2 >= total
}

func tryBase64(s string) (string, bool) {
	clean := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, s)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(clean); err == nil {
			return string(b), true
		}
	}
	return "", false
}

// ErrUnknownFormat is returned when the input matches no supported format.
var ErrUnknownFormat = errors.New("unrecognized profile format")

// Convert detects the format of data and converts it into a mihomo config.
func Convert(data []byte, opt Options) (*Result, error) {
	data = bytes.TrimPrefix(data, []byte("\ufeff"))
	format := Detect(data)
	var (
		p   *Profile
		err error
	)
	switch format {
	case FormatSurge, FormatSurgeList:
		p, err = ParseSurge(string(data), opt)
	case FormatClash:
		return convertClash(data, opt)
	case FormatURIList:
		p = newProfile()
		text := strings.TrimSpace(string(data))
		if dec, ok := tryBase64(text); ok && strings.Contains(dec, "://") {
			text = dec
		}
		for _, px := range ParseURIList(text, p.warn) {
			p.addProxy(px)
		}
	case FormatSingBox:
		p, err = ParseSingBox(data)
	default:
		return nil, ErrUnknownFormat
	}
	if err != nil {
		return nil, err
	}
	if opt.TestURL != "" {
		p.TestURL = opt.TestURL
	}
	if len(p.Proxies) == 0 && p.Providers.Len() == 0 && len(p.Groups) == 0 {
		return nil, fmt.Errorf("no usable proxies found (%s)", format)
	}
	if err := p.finishRules(opt); err != nil {
		return nil, err
	}
	out, err := p.Render()
	if err != nil {
		return nil, err
	}
	return &Result{
		Format:   format,
		YAML:     out,
		Warnings: nonNil(p.warnings),
		Proxies:  len(p.Proxies),
		Groups:   len(p.Groups),
		Rules:    len(p.Rules),
		Managed:  p.Managed,
	}, nil
}

// finishRules applies the rule template override, or the built-in default
// rules when the source has none.
func (p *Profile) finishRules(opt Options) error {
	if opt.Override != nil && strings.TrimSpace(opt.Override.Template) != "" {
		t, err := ParseTemplate(opt.Override.Template)
		if err != nil {
			return fmt.Errorf("rule template: %w", err)
		}
		p.ApplyOverride(t, opt.Override.Mode)
		return nil
	}
	if len(p.Rules) == 0 || opt.ForceTemplate {
		applyTemplate(p)
		p.pruneRuleProviders()
	}
	return nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// Render serialises the profile into mihomo YAML.
func (p *Profile) Render() (string, error) {
	root := NewOMap()
	root.Set("mixed-port", 7890)
	root.Set("allow-lan", false)
	root.Set("mode", "rule")
	root.Set("log-level", "info")
	for _, k := range p.Top.Keys() {
		v, _ := p.Top.Get(k)
		root.Set(k, v)
	}
	if _, ok := root.Get("ipv6"); !ok {
		root.Set("ipv6", true)
	}
	if p.DNS == nil {
		p.DNS = DefaultDNS()
	}
	root.Set("dns", p.DNS)
	if p.Hosts.Len() > 0 {
		root.Set("hosts", p.Hosts)
	}
	root.Set("proxies", nonNilOMaps(p.Proxies))
	if p.Providers.Len() > 0 {
		root.Set("proxy-providers", p.Providers)
	}
	root.Set("proxy-groups", nonNilOMaps(p.Groups))
	if p.RuleProviders.Len() > 0 {
		root.Set("rule-providers", p.RuleProviders)
	}
	root.Set("rules", p.Rules)

	var buf bytes.Buffer
	buf.WriteString("# Generated by Surfmi converter. Edit the source profile instead of this file.\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return "", err
	}
	_ = enc.Close()
	return buf.String(), nil
}

func nonNilOMaps(s []*OMap) []*OMap {
	if s == nil {
		return []*OMap{}
	}
	return s
}

// DefaultDNS returns the DNS block used when a source has none.
func DefaultDNS() *OMap {
	d := NewOMap()
	d.Set("enable", true)
	d.Set("ipv6", true)
	d.Set("enhanced-mode", "fake-ip")
	d.Set("fake-ip-range", "198.18.0.1/16")
	d.Set("fake-ip-filter", []string{
		"*.lan", "+.local", "+.msftconnecttest.com", "+.msftncsi.com",
		"time.*.com", "+.ntp.org", "+.pool.ntp.org", "localhost.ptlogin2.qq.com",
		"+.stun.*.*", "+.stun.*.*.*",
	})
	d.Set("default-nameserver", []string{"223.5.5.5", "119.29.29.29"})
	d.Set("nameserver", []string{"https://dns.alidns.com/dns-query", "https://doh.pub/dns-query"})
	d.Set("proxy-server-nameserver", []string{"https://dns.alidns.com/dns-query", "https://doh.pub/dns-query"})
	return d
}
