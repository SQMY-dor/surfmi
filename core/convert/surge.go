// SPDX-License-Identifier: GPL-3.0-or-later

package convert

import (
	"fmt"
	"net/netip"
	"regexp"
	"strings"
)

// unsupportedSections are Surge features that rely on Surge's own engine
// (HTTP(S) interception, JavaScript runtime, ...) and have no mihomo
// equivalent. They are reported, not converted.
var unsupportedSections = map[string]string{
	"url rewrite":    "URL rewriting requires HTTP interception (MITM)",
	"header rewrite": "header rewriting requires HTTP interception (MITM)",
	"body rewrite":   "body rewriting requires HTTP interception (MITM)",
	"mitm":           "MITM (HTTPS decryption) is not supported",
	"script":         "scripts need Surge's JavaScript runtime",
	"map local":      "Map Local requires HTTP interception",
	"panel":          "panels are a Surge UI feature",
	"ssid setting":   "SSID settings are not supported",
	"replica":        "Replica settings are not supported",
	"snell server":   "Snell server mode is not supported",
	"proxy chain":    "proxy chains are not supported (use underlying-proxy)",
}

// ParseSurge converts a Surge profile (or a bare Surge proxy list, as served
// by policy-path) into a Profile.
func ParseSurge(text string, opt Options) (*Profile, error) {
	p := newProfile()
	p.TestURL = DefaultTestURL
	return parseSurgeInto(p, text, opt)
}

func parseSurgeInto(p *Profile, text string, opt Options) (*Profile, error) {
	sections := map[string][]numberedLine{}
	var order []string
	cur := "proxy" // bare lists (policy-path content) are proxy lines
	reported := map[string]bool{}

	for i, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "#!MANAGED-CONFIG") {
			p.Managed = parseManaged(line)
			continue
		}
		if line == "" || isComment(line) {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			cur = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			if _, seen := sections[cur]; !seen {
				order = append(order, cur)
				sections[cur] = nil
			}
			continue
		}
		sections[cur] = append(sections[cur], numberedLine{i + 1, line})
	}

	for _, sec := range order {
		if why, ok := unsupportedSections[sec]; ok && len(sections[sec]) > 0 && !reported[sec] {
			reported[sec] = true
			p.warn("[%s] ignored: %s on Android (%d lines)", sectionTitle(sec), why, len(sections[sec]))
		}
	}

	wg := map[string][]numberedLine{}
	for sec, lines := range sections {
		if strings.HasPrefix(sec, "wireguard ") {
			wg[strings.TrimSpace(sec[len("wireguard "):])] = lines
		}
	}

	parseSurgeGeneral(p, sections["general"])
	parseSurgeHosts(p, sections["host"])
	for _, l := range sections["proxy"] {
		parseSurgeProxyLine(p, l, wg)
	}
	for _, l := range sections["proxy group"] {
		parseSurgeGroupLine(p, l, opt)
	}
	parseSurgeRules(p, sections["rule"])
	return p, nil
}

type numberedLine struct {
	n    int
	text string
}

func sectionTitle(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if w == "mitm" || w == "url" || w == "ssid" {
			words[i] = strings.ToUpper(w)
		} else if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func parseManaged(line string) *Managed {
	fields := strings.Fields(strings.TrimPrefix(line, "#!MANAGED-CONFIG"))
	m := &Managed{}
	for _, f := range fields {
		switch {
		case strings.HasPrefix(f, "http://"), strings.HasPrefix(f, "https://"):
			m.URL = f
		case strings.HasPrefix(f, "interval="):
			m.Interval = atoi(strings.TrimPrefix(f, "interval="))
		case strings.HasPrefix(f, "strict="):
			m.Strict = parseBool(strings.TrimPrefix(f, "strict="))
		}
	}
	return m
}

// ---------------------------------------------------------------- [General]

func parseSurgeGeneral(p *Profile, lines []numberedLine) {
	var plainDNS, encDNS []string
	var realIP []string
	for _, l := range lines {
		k, v, ok := splitKV(l.text)
		if !ok {
			continue
		}
		switch strings.ToLower(k) {
		case "ipv6":
			p.Top.Set("ipv6", parseBool(v))
		case "loglevel":
			p.Top.Set("log-level", map[string]string{
				"verbose": "debug", "info": "info", "notify": "info", "warning": "warning",
			}[strings.ToLower(v)])
		case "allow-wifi-access":
			p.Top.Set("allow-lan", parseBool(v))
		case "proxy-test-url":
			if v != "" {
				p.TestURL = v
			}
		case "dns-server":
			for _, s := range splitList(v) {
				if strings.EqualFold(s, "system") {
					continue
				}
				plainDNS = append(plainDNS, s)
			}
		case "encrypted-dns-server":
			encDNS = append(encDNS, splitList(v)...)
		case "always-real-ip":
			for _, s := range splitList(v) {
				realIP = append(realIP, surgeDomainPattern(s))
			}
		case "tun-excluded-routes", "bypass-tun":
			p.warn("[General] %s is handled by the Android VPN routing settings, ignored", k)
		}
	}
	if len(plainDNS) == 0 && len(encDNS) == 0 && len(realIP) == 0 {
		return
	}
	d := DefaultDNS()
	if len(plainDNS) > 0 {
		d.Set("default-nameserver", plainDNS)
	}
	if len(encDNS) > 0 {
		d.Set("nameserver", encDNS)
		d.Set("proxy-server-nameserver", encDNS)
	} else if len(plainDNS) > 0 {
		d.Set("nameserver", plainDNS)
		d.Set("proxy-server-nameserver", plainDNS)
	}
	if len(realIP) > 0 {
		cur, _ := d.Get("fake-ip-filter")
		d.Set("fake-ip-filter", uniqueStrings(append(cur.([]string), realIP...)))
	}
	p.DNS = d
}

// surgeDomainPattern converts Surge wildcard patterns to mihomo ones.
// Surge "*.example.com" matches subdomains, mihomo "+.example.com" matches the
// domain and its subdomains; "*" is kept as-is because mihomo supports it.
func surgeDomainPattern(s string) string {
	s = strings.TrimPrefix(s, "-") // "-*.foo" exclusion markers: best effort
	return s
}

// ---------------------------------------------------------------- [Host]

func parseSurgeHosts(p *Profile, lines []numberedLine) {
	policy := NewOMap()
	for _, l := range lines {
		k, v, ok := splitKV(l.text)
		if !ok || k == "" || v == "" {
			continue
		}
		if strings.HasPrefix(v, "server:") {
			srv := strings.TrimPrefix(v, "server:")
			if strings.EqualFold(srv, "system") {
				continue
			}
			pat := k
			if strings.HasPrefix(pat, "*.") {
				pat = "+." + pat[2:]
			}
			policy.Set(pat, srv)
			continue
		}
		if v == "" {
			continue
		}
		p.Hosts.Set(k, v)
	}
	if policy.Len() > 0 {
		if p.DNS == nil {
			p.DNS = DefaultDNS()
		}
		p.DNS.Set("nameserver-policy", policy)
	}
}

// ---------------------------------------------------------------- [Proxy]

func parseSurgeProxyLine(p *Profile, l numberedLine, wg map[string][]numberedLine) {
	name, rest, ok := splitKV(l.text)
	if !ok {
		p.warn("line %d: not a proxy definition, skipped", l.n)
		return
	}
	name = unquote(name)
	tokens := splitTop(rest, ',')
	typ := strings.ToLower(tokens[0])
	kv := parseKVTokens(tokens[1:])
	px, err := surgeProxy(name, typ, kv, wg)
	if err != nil {
		p.warn("line %d: proxy %q skipped: %v", l.n, name, err)
		return
	}
	if px == nil {
		return
	}
	p.addProxy(px)
}

func surgeProxy(name, typ string, kv *kvList, wg map[string][]numberedLine) (*OMap, error) {
	px := NewOMap().Set("name", name)
	server, port := "", 0
	if len(kv.pos) >= 2 {
		server, port = kv.pos[0], atoi(kv.pos[1])
	}
	needServer := func() error {
		if server == "" || port <= 0 {
			return fmt.Errorf("missing server or port")
		}
		px.Set("server", server).Set("port", port)
		return nil
	}

	switch typ {
	case "direct":
		return px.Set("type", "direct"), nil
	case "reject", "reject-drop", "reject-tinygif", "reject-no-drop":
		return px.Set("type", "reject"), nil
	case "ss", "custom":
		px.Set("type", "ss")
		if err := needServer(); err != nil {
			return nil, err
		}
		cipher := kv.get("encrypt-method")
		password := kv.get("password")
		if typ == "custom" && len(kv.pos) >= 4 {
			cipher, password = kv.pos[2], kv.pos[3]
		}
		if cipher == "" {
			return nil, fmt.Errorf("missing encrypt-method")
		}
		px.Set("cipher", cipher).Set("password", password)
		if obfs := kv.get("obfs"); obfs == "http" || obfs == "tls" {
			opts := NewOMap().Set("mode", obfs)
			opts.SetStr("host", kv.get("obfs-host"))
			px.Set("plugin", "obfs").Set("plugin-opts", opts)
		}
		if pw := kv.get("shadow-tls-password"); pw != "" {
			opts := NewOMap().SetStr("host", kv.get("shadow-tls-sni")).Set("password", pw)
			v := atoi(kv.get("shadow-tls-version"))
			if v == 0 {
				v = 3
			}
			opts.Set("version", v)
			px.Set("plugin", "shadow-tls").Set("client-fingerprint", "chrome").Set("plugin-opts", opts)
		}
		setUDP(px, kv, false)
	case "vmess":
		px.Set("type", "vmess")
		if err := needServer(); err != nil {
			return nil, err
		}
		uuid := kv.get("username")
		if uuid == "" {
			return nil, fmt.Errorf("missing username (uuid)")
		}
		px.Set("uuid", uuid).Set("alterId", 0).Set("cipher", orDefault(kv.get("encrypt-method"), "auto"))
		if b, ok := kv.bool("tls"); ok && b {
			px.Set("tls", true)
			setSNI(px, kv, "servername")
		}
		setWS(px, kv)
		setUDP(px, kv, true)
	case "trojan":
		px.Set("type", "trojan")
		if err := needServer(); err != nil {
			return nil, err
		}
		px.Set("password", kv.get("password"))
		setSNI(px, kv, "sni")
		setWS(px, kv)
		setUDP(px, kv, true)
	case "snell":
		px.Set("type", "snell")
		if err := needServer(); err != nil {
			return nil, err
		}
		px.Set("psk", kv.get("psk"))
		ver := atoi(kv.get("version"))
		if ver == 0 {
			ver = 1
		}
		px.Set("version", ver)
		if obfs := kv.get("obfs"); obfs == "http" || obfs == "tls" {
			opts := NewOMap().Set("mode", obfs)
			opts.SetStr("host", kv.get("obfs-host"))
			px.Set("obfs-opts", opts)
		}
		if b, ok := kv.bool("reuse"); ok && b && (ver == 4 || ver == 6) {
			px.Set("reuse", true)
		}
		if ver >= 3 {
			setUDP(px, kv, false)
		}
	case "http", "https":
		px.Set("type", "http")
		if err := needServer(); err != nil {
			return nil, err
		}
		setUserPass(px, kv)
		if typ == "https" {
			px.Set("tls", true)
			setSNI(px, kv, "sni")
		}
	case "socks5", "socks5-tls":
		px.Set("type", "socks5")
		if err := needServer(); err != nil {
			return nil, err
		}
		setUserPass(px, kv)
		if typ == "socks5-tls" {
			px.Set("tls", true)
		}
		setUDP(px, kv, true)
	case "tuic", "tuic-v5":
		px.Set("type", "tuic")
		if err := needServer(); err != nil {
			return nil, err
		}
		if typ == "tuic" && kv.get("token") != "" {
			px.Set("token", kv.get("token"))
		} else {
			px.Set("uuid", kv.get("uuid")).Set("password", kv.get("password"))
		}
		if alpn := splitList(kv.get("alpn")); len(alpn) > 0 {
			px.Set("alpn", alpn)
		} else {
			px.Set("alpn", []string{"h3"})
		}
		setSNI(px, kv, "sni")
		px.Set("udp-relay-mode", "native")
	case "hysteria2":
		px.Set("type", "hysteria2")
		if err := needServer(); err != nil {
			return nil, err
		}
		px.Set("password", kv.get("password"))
		if bw := kv.get("download-bandwidth"); bw != "" {
			px.Set("down", bw+" Mbps")
		}
		if hop := kv.get("port-hopping"); hop != "" {
			px.Set("ports", strings.ReplaceAll(hop, ";", ","))
			if iv, ok := kv.int("port-hopping-interval"); ok {
				px.Set("hop-interval", iv)
			}
		}
		if pw := kv.get("salamander-password"); pw != "" {
			px.Set("obfs", "salamander").Set("obfs-password", pw)
		}
		setSNI(px, kv, "sni")
	case "anytls":
		px.Set("type", "anytls")
		if err := needServer(); err != nil {
			return nil, err
		}
		px.Set("password", kv.get("password"))
		setSNI(px, kv, "sni")
		px.Set("client-fingerprint", "chrome")
		setUDP(px, kv, true)
	case "ssh":
		px.Set("type", "ssh")
		if err := needServer(); err != nil {
			return nil, err
		}
		px.SetStr("username", kv.get("username"))
		px.SetStr("password", kv.get("password"))
		if kv.get("private-key") != "" {
			return nil, fmt.Errorf("ssh private-key references a Surge keystore entry, not supported")
		}
	case "wireguard":
		return surgeWireGuard(px, kv, wg)
	case "external":
		return nil, fmt.Errorf("external proxy programs are not supported on Android")
	default:
		return nil, fmt.Errorf("unsupported proxy type %q", typ)
	}

	setCommon(px, kv)
	return px, nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func setUserPass(px *OMap, kv *kvList) {
	user, pass := kv.get("username"), kv.get("password")
	if user == "" && len(kv.pos) >= 4 {
		user, pass = kv.pos[2], kv.pos[3]
	}
	px.SetStr("username", user)
	px.SetStr("password", pass)
}

func setUDP(px *OMap, kv *kvList, def bool) {
	if b, ok := kv.bool("udp-relay"); ok {
		px.Set("udp", b)
	} else if def {
		px.Set("udp", true)
	}
}

func setSNI(px *OMap, kv *kvList, key string) {
	if sni := kv.get("sni"); sni != "" && !strings.EqualFold(sni, "off") {
		px.Set(key, sni)
	}
}

func setWS(px *OMap, kv *kvList) {
	if b, _ := kv.bool("ws"); !b {
		return
	}
	px.Set("network", "ws")
	opts := NewOMap().Set("path", orDefault(kv.get("ws-path"), "/"))
	if h := kv.get("ws-headers"); h != "" {
		headers := NewOMap()
		for _, part := range strings.Split(h, "|") {
			if i := strings.Index(part, ":"); i > 0 {
				headers.Set(strings.TrimSpace(part[:i]), strings.TrimSpace(part[i+1:]))
			}
		}
		if headers.Len() > 0 {
			opts.Set("headers", headers)
		}
	}
	px.Set("ws-opts", opts)
}

// setCommon maps options shared by all Surge proxy types.
func setCommon(px *OMap, kv *kvList) {
	if b, ok := kv.bool("skip-cert-verify"); ok && b {
		px.Set("skip-cert-verify", true)
	}
	if b, ok := kv.bool("tfo"); ok && b {
		px.Set("tfo", true)
	}
	if u := kv.get("underlying-proxy"); u != "" {
		px.Set("dialer-proxy", u)
	}
	if fp := kv.get("server-cert-fingerprint-sha256"); fp != "" {
		px.Set("fingerprint", strings.ReplaceAll(fp, ":", ""))
	}
	switch strings.ToLower(kv.get("ip-version")) {
	case "v4-only":
		px.Set("ip-version", "ipv4")
	case "v6-only":
		px.Set("ip-version", "ipv6")
	case "prefer-v4":
		px.Set("ip-version", "ipv4-prefer")
	case "prefer-v6":
		px.Set("ip-version", "ipv6-prefer")
	case "dual":
		px.Set("ip-version", "dual")
	}
}

func surgeWireGuard(px *OMap, kv *kvList, wg map[string][]numberedLine) (*OMap, error) {
	secName := kv.get("section-name")
	lines, ok := wg[strings.ToLower(secName)]
	if !ok {
		return nil, fmt.Errorf("[WireGuard %s] section not found", secName)
	}
	px.Set("type", "wireguard")
	var peers []*OMap
	conf := map[string]string{}
	for _, l := range lines {
		k, v, ok := splitKV(l.text)
		if !ok {
			continue
		}
		k = strings.ToLower(k)
		if k == "peer" {
			peers = append(peers, parseWGPeer(v))
			continue
		}
		conf[k] = v
	}
	if len(peers) == 0 {
		return nil, fmt.Errorf("wireguard section has no peer")
	}
	px.Set("private-key", conf["private-key"])
	if ip := conf["self-ip"]; ip != "" {
		px.Set("ip", ip)
	}
	if ip := conf["self-ip-v6"]; ip != "" {
		px.Set("ipv6", ip)
	}
	if mtu := atoi(conf["mtu"]); mtu > 0 {
		px.Set("mtu", mtu)
	}
	if dns := splitList(conf["dns-server"]); len(dns) > 0 {
		px.Set("dns", dns)
		px.Set("remote-dns-resolve", true)
	}
	px.Set("udp", true)
	if len(peers) == 1 {
		pe := peers[0]
		for _, k := range pe.Keys() {
			v, _ := pe.Get(k)
			px.Set(k, v)
		}
	} else {
		px.Set("server", peers[0].GetString("server"))
		px.Set("port", peers[0].vals["port"])
		px.Set("peers", peers)
	}
	setCommon(px, kv)
	return px, nil
}

func parseWGPeer(v string) *OMap {
	v = strings.TrimSpace(v)
	v = strings.TrimSuffix(strings.TrimPrefix(v, "("), ")")
	pe := NewOMap()
	for _, part := range splitTop(v, ',') {
		k, val, ok := splitKV(part)
		if !ok {
			continue
		}
		switch strings.ToLower(k) {
		case "endpoint":
			host, port := splitHostPort(val)
			pe.Set("server", host).Set("port", port)
		case "public-key":
			pe.Set("public-key", val)
		case "preshared-key":
			pe.Set("pre-shared-key", val)
		case "allowed-ips":
			pe.Set("allowed-ips", splitList(val))
		case "client-id":
			var r []int
			for _, x := range strings.Split(val, "/") {
				r = append(r, atoi(x))
			}
			pe.Set("reserved", r)
		case "keepalive":
			pe.Set("persistent-keepalive", atoi(val))
		}
	}
	return pe
}

func splitHostPort(s string) (string, int) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[") {
		if i := strings.Index(s, "]"); i > 0 {
			return s[1:i], atoi(strings.TrimPrefix(s[i+1:], ":"))
		}
	}
	if i := strings.LastIndex(s, ":"); i > 0 {
		return s[:i], atoi(s[i+1:])
	}
	return s, 0
}

// ---------------------------------------------------------------- [Proxy Group]

func mapPolicy(n string) string {
	switch strings.ToUpper(n) {
	case "DIRECT":
		return "DIRECT"
	case "REJECT", "REJECT-TINYGIF", "REJECT-NO-DROP", "REJECT-IMG", "REJECT-DICT", "REJECT-ARRAY":
		return "REJECT"
	case "REJECT-DROP":
		return "REJECT-DROP"
	}
	return n
}

var groupOptionKeys = map[string]bool{
	"url": true, "interval": true, "tolerance": true, "timeout": true, "policy-path": true,
	"update-interval": true, "policy-regex-filter": true, "include-all-proxies": true,
	"include-other-group": true, "hidden": true, "no-alert": true, "persistent": true,
	"evaluate-before-use": true, "max-timeout": true, "external-policy-modifier": true,
	"icon-url": true, "default": true, "policy-select-name": true,
}

func parseSurgeGroupLine(p *Profile, l numberedLine, opt Options) {
	name, rest, ok := splitKV(l.text)
	if !ok {
		p.warn("line %d: not a group definition, skipped", l.n)
		return
	}
	name = unquote(name)
	tokens := splitTop(rest, ',')
	typ := strings.ToLower(tokens[0])

	var members []string
	opts := map[string]string{}
	for _, t := range tokens[1:] {
		if t == "" {
			continue
		}
		if k, v, ok := splitKV(t); ok && groupOptionKeys[strings.ToLower(k)] {
			opts[strings.ToLower(k)] = v
			continue
		}
		if k, v, ok := splitKV(t); ok && typ == "subnet" {
			// "SSID:home" = PolicyA  -> member PolicyA
			_ = k
			members = append(members, mapPolicy(unquote(v)))
			continue
		}
		members = append(members, mapPolicy(unquote(t)))
	}

	g := NewOMap().Set("name", name)
	switch typ {
	case "select":
		g.Set("type", "select")
	case "url-test", "smart":
		g.Set("type", "url-test")
		if typ == "smart" {
			p.warn("line %d: group %q type smart is mapped to url-test", l.n, name)
		}
	case "fallback":
		g.Set("type", "fallback")
	case "load-balance":
		g.Set("type", "load-balance")
		if parseBool(opts["persistent"]) {
			g.Set("strategy", "consistent-hashing")
		} else {
			g.Set("strategy", "round-robin")
		}
	case "subnet":
		g.Set("type", "select")
		if d := opts["default"]; d != "" {
			members = append([]string{mapPolicy(d)}, members...)
		}
		p.warn("line %d: subnet group %q converted to a manual select group (network-based switching is not supported)", l.n, name)
	default:
		p.warn("line %d: group %q has unsupported type %q, converted to select", l.n, name, typ)
		g.Set("type", "select")
	}

	if other := opts["include-other-group"]; other != "" {
		members = append(members, splitList(other)...)
	}

	filterRe := opts["policy-regex-filter"]
	if path := opts["policy-path"]; path != "" {
		names, used := p.loadPolicyPath(name, path, filterRe, atoi(opts["update-interval"]), opt)
		members = append(members, names...)
		if used != "" {
			g.Set("use", []string{used})
		}
	}
	includeAll := parseBool(opts["include-all-proxies"])
	if p.template && filterRe != "" && opts["policy-path"] == "" {
		// in a rule template a bare policy-regex-filter selects from the
		// subscription's proxies
		includeAll = true
	}
	if includeAll {
		g.Set("include-all-proxies", true)
	}
	if filterRe != "" {
		if _, ok := g.Get("use"); ok || includeAll {
			g.Set("filter", filterRe)
		}
	}

	members = uniqueStrings(members)
	_, hasUse := g.Get("use")
	if len(members) == 0 && !hasUse && !includeAll {
		p.warn("line %d: group %q has no members, DIRECT added", l.n, name)
		members = []string{"DIRECT"}
	}
	if len(members) > 0 {
		g.Set("proxies", members)
	}

	t := g.GetString("type")
	if t == "url-test" || t == "fallback" || t == "load-balance" {
		g.Set("url", orDefault(opts["url"], p.TestURL))
		iv := atoi(opts["interval"])
		if iv <= 0 {
			iv = 600
		}
		g.Set("interval", iv)
		if t == "url-test" {
			if tol := atoi(opts["tolerance"]); tol > 0 {
				g.Set("tolerance", tol)
			}
		}
		if to := atoi(opts["timeout"]); to > 0 {
			g.Set("timeout", to*1000)
		}
		g.Set("lazy", true)
	}
	if parseBool(opts["hidden"]) {
		g.Set("hidden", true)
	}
	if icon := opts["icon-url"]; icon != "" {
		g.Set("icon", icon)
	}
	p.Groups = append(p.Groups, g)
}

// loadPolicyPath fetches a Surge policy-path list and inlines its proxies.
// When fetching is impossible it emits a mihomo proxy-provider instead and
// returns its name as `used`.
func (p *Profile) loadPolicyPath(group, url, filter string, interval int, opt Options) (names []string, used string) {
	var re *regexp.Regexp
	if filter != "" {
		var err error
		if re, err = regexp.Compile(filter); err != nil {
			p.warn("group %q: policy-regex-filter %q is not a valid Go regexp, ignored for inline proxies", group, filter)
		}
	}
	if opt.Fetcher != nil {
		data, err := opt.Fetcher.Fetch(url)
		if err == nil {
			sub := newProfile()
			switch Detect(data) {
			case FormatSurge, FormatSurgeList:
				sp, _ := ParseSurge(string(data), Options{})
				sub = sp
			case FormatURIList:
				text := strings.TrimSpace(string(data))
				if dec, ok := tryBase64(text); ok && strings.Contains(dec, "://") {
					text = dec
				}
				for _, px := range ParseURIList(text, sub.warn) {
					sub.addProxy(px)
				}
			case FormatClash:
				if cp, err := clashToProfile(data); err == nil {
					sub = cp
				}
			}
			for _, w := range sub.warnings {
				p.warn("policy-path %s: %s", url, w)
			}
			for _, px := range sub.Proxies {
				if re != nil && !re.MatchString(px.GetString("name")) {
					continue
				}
				names = append(names, p.addProxy(px))
			}
			if len(names) == 0 {
				p.warn("group %q: policy-path returned no matching proxies", group)
			}
			return names, ""
		}
		p.warn("group %q: failed to fetch policy-path (%v); using a runtime proxy-provider", group, err)
	}
	prov := "pp_" + sanitizeName(group)
	if interval <= 0 {
		interval = 86400
	}
	pv := NewOMap().Set("type", "http").Set("url", url).
		Set("path", "./providers/"+prov+".yaml").Set("interval", interval)
	pv.Set("health-check", NewOMap().Set("enable", true).Set("url", p.TestURL).Set("interval", 600))
	p.Providers.Set(prov, pv)
	p.warn("group %q: policy-path is loaded at runtime by mihomo; it must serve Clash YAML or a URI list", group)
	return nil, prov
}

// ---------------------------------------------------------------- [Rule]

var (
	lanCIDRs4 = []string{"10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.168.0.0/16", "224.0.0.0/4"}
	lanCIDRs6 = []string{"::1/128", "fc00::/7", "fe80::/10"}
)

func parseSurgeRules(p *Profile, lines []numberedLine) {
	processWarned := false
	for _, l := range lines {
		rules, err := convertSurgeRule(p, l.text)
		if err != nil {
			p.warn("line %d: rule %q skipped: %v", l.n, truncate(l.text, 80), err)
			continue
		}
		if !processWarned && strings.HasPrefix(strings.ToUpper(l.text), "PROCESS-NAME") {
			processWarned = true
			p.warn("PROCESS-NAME rules match Android package names (e.g. com.android.chrome), not desktop process names")
		}
		p.Rules = append(p.Rules, rules...)
		if len(rules) > 0 && strings.HasPrefix(rules[len(rules)-1], "MATCH,") {
			break
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

var ruleTypeMap = map[string]string{
	"DOMAIN": "DOMAIN", "DOMAIN-SUFFIX": "DOMAIN-SUFFIX", "DOMAIN-KEYWORD": "DOMAIN-KEYWORD",
	"DOMAIN-WILDCARD": "DOMAIN-WILDCARD", "DOMAIN-REGEX": "DOMAIN-REGEX",
	"IP-CIDR": "IP-CIDR", "IP-CIDR6": "IP-CIDR6", "GEOIP": "GEOIP", "GEOSITE": "GEOSITE", "IP-ASN": "IP-ASN",
	"SRC-IP": "SRC-IP-CIDR", "SRC-IP-CIDR": "SRC-IP-CIDR",
	"DEST-PORT": "DST-PORT", "DST-PORT": "DST-PORT", "SRC-PORT": "SRC-PORT", "IN-PORT": "IN-PORT",
	"PROCESS-NAME": "PROCESS-NAME",
}

var unsupportedRuleTypes = map[string]string{
	"USER-AGENT": "requires HTTP inspection", "URL-REGEX": "requires HTTP inspection",
	"SUBNET": "network-based matching is not supported", "CELLULAR-RADIO": "not supported",
	"DEVICE-NAME": "not supported", "HOSTNAME-TYPE": "not supported", "SCRIPT": "scripts are not supported",
	"IN-INTERFACE": "not supported",
}

// convertSurgeRule converts one Surge rule line to one or more mihomo rules.
func convertSurgeRule(p *Profile, line string) ([]string, error) {
	tokens := splitTop(line, ',')
	typ := strings.ToUpper(tokens[0])
	switch typ {
	case "FINAL":
		if len(tokens) < 2 {
			return nil, fmt.Errorf("FINAL without policy")
		}
		return []string{"MATCH," + mapPolicy(unquote(tokens[1]))}, nil
	case "RULE-SET", "DOMAIN-SET":
		if len(tokens) < 3 {
			return nil, fmt.Errorf("missing url or policy")
		}
		src, policy := unquote(tokens[1]), mapPolicy(unquote(tokens[2]))
		noResolve := hasOpt(tokens[3:], "no-resolve")
		if typ == "RULE-SET" && strings.EqualFold(src, "LAN") {
			var out []string
			for _, c := range lanCIDRs4 {
				out = append(out, "IP-CIDR,"+c+","+policy+",no-resolve")
			}
			for _, c := range lanCIDRs6 {
				out = append(out, "IP-CIDR6,"+c+","+policy+",no-resolve")
			}
			return append([]string{"DOMAIN-SUFFIX,local," + policy}, out...), nil
		}
		if typ == "RULE-SET" && strings.EqualFold(src, "SYSTEM") {
			return nil, fmt.Errorf("built-in SYSTEM rule set has no Android equivalent")
		}
		if !strings.Contains(src, "://") {
			return nil, fmt.Errorf("local rule set files are not supported, use a URL")
		}
		behavior := "classical"
		if typ == "DOMAIN-SET" {
			behavior = "domain"
		}
		name := p.ruleProvider(src, behavior)
		r := "RULE-SET," + name + "," + policy
		if noResolve {
			r += ",no-resolve"
		}
		return []string{r}, nil
	case "AND", "OR", "NOT":
		if len(tokens) < 3 {
			return nil, fmt.Errorf("malformed logical rule")
		}
		inner, err := convertLogical(tokens[1])
		if err != nil {
			return nil, err
		}
		return []string{typ + "," + inner + "," + mapPolicy(unquote(tokens[2]))}, nil
	case "PROTOCOL":
		if len(tokens) < 3 {
			return nil, fmt.Errorf("malformed PROTOCOL rule")
		}
		policy := mapPolicy(unquote(tokens[2]))
		switch strings.ToUpper(tokens[1]) {
		case "TCP":
			return []string{"NETWORK,tcp," + policy}, nil
		case "UDP":
			return []string{"NETWORK,udp," + policy}, nil
		case "QUIC":
			return []string{"AND,((NETWORK,udp),(DST-PORT,443))," + policy}, nil
		}
		return nil, fmt.Errorf("PROTOCOL %s is not supported", tokens[1])
	}
	if why, ok := unsupportedRuleTypes[typ]; ok {
		return nil, fmt.Errorf("%s %s", typ, why)
	}
	body, err := convertRuleBody(tokens)
	if err != nil {
		return nil, err
	}
	if len(tokens) < 3 {
		return nil, fmt.Errorf("missing policy")
	}
	r := body + "," + mapPolicy(unquote(tokens[2]))
	if hasOpt(tokens[3:], "no-resolve") {
		r += ",no-resolve"
	}
	return []string{r}, nil
}

func hasOpt(tokens []string, opt string) bool {
	for _, t := range tokens {
		if strings.EqualFold(strings.TrimSpace(t), opt) {
			return true
		}
	}
	return false
}

// convertRuleBody converts "TYPE,value" (without policy).
func convertRuleBody(tokens []string) (string, error) {
	typ := strings.ToUpper(tokens[0])
	mt, ok := ruleTypeMap[typ]
	if !ok {
		if why, bad := unsupportedRuleTypes[typ]; bad {
			return "", fmt.Errorf("%s %s", typ, why)
		}
		return "", fmt.Errorf("unsupported rule type %s", typ)
	}
	if len(tokens) < 2 {
		return "", fmt.Errorf("missing value")
	}
	val := unquote(tokens[1])
	switch mt {
	case "DST-PORT", "SRC-PORT", "IN-PORT":
		v, err := convertPortExpr(val)
		if err != nil {
			return "", err
		}
		val = v
	case "SRC-IP-CIDR":
		if !strings.Contains(val, "/") {
			if a, err := netip.ParseAddr(val); err == nil {
				val = netip.PrefixFrom(a, a.BitLen()).String()
			}
		}
	case "IP-ASN":
		val = strings.TrimPrefix(strings.ToUpper(val), "AS")
	}
	return mt + "," + val, nil
}

// convertPortExpr turns Surge port expressions (">=1024", "80-90", "443")
// into mihomo port ranges.
func convertPortExpr(v string) (string, error) {
	v = strings.TrimSpace(v)
	switch {
	case strings.HasPrefix(v, ">="):
		return strings.TrimSpace(v[2:]) + "-65535", nil
	case strings.HasPrefix(v, "<="):
		return "0-" + strings.TrimSpace(v[2:]), nil
	case strings.HasPrefix(v, ">"):
		return fmt.Sprintf("%d-65535", atoi(v[1:])+1), nil
	case strings.HasPrefix(v, "<"):
		return fmt.Sprintf("0-%d", atoi(v[1:])-1), nil
	}
	return v, nil
}

// convertLogical converts "((A,b),(C,d))" recursively.
func convertLogical(group string) (string, error) {
	g := strings.TrimSpace(group)
	if !strings.HasPrefix(g, "(") || !strings.HasSuffix(g, ")") {
		return "", fmt.Errorf("malformed logical rule body")
	}
	g = g[1 : len(g)-1]
	var parts []string
	for _, sub := range splitTop(g, ',') {
		sub = strings.TrimSpace(sub)
		if !strings.HasPrefix(sub, "(") || !strings.HasSuffix(sub, ")") {
			return "", fmt.Errorf("malformed sub rule %q", sub)
		}
		tokens := splitTop(sub[1:len(sub)-1], ',')
		t := strings.ToUpper(tokens[0])
		if t == "AND" || t == "OR" || t == "NOT" {
			if len(tokens) < 2 {
				return "", fmt.Errorf("malformed nested logical rule")
			}
			inner, err := convertLogical(tokens[1])
			if err != nil {
				return "", err
			}
			parts = append(parts, "("+t+","+inner+")")
			continue
		}
		if t == "PROTOCOL" && len(tokens) >= 2 {
			switch strings.ToUpper(tokens[1]) {
			case "TCP", "UDP":
				parts = append(parts, "(NETWORK,"+strings.ToLower(tokens[1])+")")
				continue
			}
		}
		body, err := convertRuleBody(tokens)
		if err != nil {
			return "", err
		}
		parts = append(parts, "("+body+")")
	}
	return "(" + strings.Join(parts, ",") + ")", nil
}

// ruleProvider registers (or reuses) a rule-provider for url.
func (p *Profile) ruleProvider(url, behavior string) string {
	for _, k := range p.RuleProviders.Keys() {
		v, _ := p.RuleProviders.Get(k)
		if om := v.(*OMap); om.GetString("url") == url && om.GetString("behavior") == behavior {
			return k
		}
	}
	base := url
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if i := strings.IndexAny(base, "?#"); i >= 0 {
		base = base[:i]
	}
	if i := strings.LastIndex(base, "."); i > 0 {
		base = base[:i]
	}
	name := sanitizeName(base)
	final := name
	for i := 2; ; i++ {
		if _, exists := p.RuleProviders.Get(final); !exists {
			break
		}
		final = fmt.Sprintf("%s_%d", name, i)
	}
	ext, format := ".list", "text"
	if behavior == "domain" {
		ext = ".txt"
	}
	lower := strings.ToLower(url)
	if i := strings.IndexAny(lower, "?#"); i >= 0 {
		lower = lower[:i]
	}
	switch {
	case strings.HasSuffix(lower, ".mrs"):
		// mihomo binary rule sets only hold domains or IP ranges
		ext, format = ".mrs", "mrs"
		if strings.Contains(lower, "geoip") || strings.Contains(lower, "/ip/") || strings.Contains(lower, "ipcidr") {
			behavior = "ipcidr"
		} else {
			behavior = "domain"
		}
	case strings.HasSuffix(lower, ".yaml"), strings.HasSuffix(lower, ".yml"):
		ext, format = ".yaml", "yaml"
	}
	p.RuleProviders.Set(final, NewOMap().
		Set("type", "http").
		Set("behavior", behavior).
		Set("format", format).
		Set("url", url).
		Set("path", "./ruleset/"+final+ext).
		Set("interval", 86400))
	return final
}
