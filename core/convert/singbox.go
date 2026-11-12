// SPDX-License-Identifier: GPL-3.0-or-later

package convert

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type sbObj map[string]any

func (o sbObj) s(k string) string {
	switch v := o[k].(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%d", int(v))
	}
	return ""
}

func (o sbObj) i(k string) int {
	if v, ok := o[k].(float64); ok {
		return int(v)
	}
	return atoi(o.s(k))
}

func (o sbObj) b(k string) bool {
	v, _ := o[k].(bool)
	return v
}

func (o sbObj) o(k string) sbObj {
	if v, ok := o[k].(map[string]any); ok {
		return v
	}
	return sbObj{}
}

func (o sbObj) strs(k string) []string {
	switch v := o[k].(type) {
	case []any:
		var out []string
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		return []string{v}
	}
	return nil
}

// ParseSingBox converts the outbounds (and wireguard endpoints) of a
// sing-box configuration. Routing rules are not converted.
func ParseSingBox(data []byte) (*Profile, error) {
	var root struct {
		Outbounds []map[string]any `json:"outbounds"`
		Endpoints []map[string]any `json:"endpoints"`
		Route     map[string]any   `json:"route"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("sing-box: %w", err)
	}
	p := newProfile()
	p.TestURL = DefaultTestURL
	alias := map[string]string{} // tag -> mihomo name
	var groups []sbObj

	all := append([]map[string]any{}, root.Outbounds...)
	all = append(all, root.Endpoints...)
	for _, raw := range all {
		ob := sbObj(raw)
		tag, typ := ob.s("tag"), ob.s("type")
		switch typ {
		case "direct":
			alias[tag] = "DIRECT"
			continue
		case "block":
			alias[tag] = "REJECT"
			continue
		case "dns":
			continue
		case "selector", "urltest":
			groups = append(groups, ob)
			continue
		}
		px, err := singBoxProxy(ob)
		if err != nil {
			p.warn("sing-box outbound %q skipped: %v", tag, err)
			continue
		}
		alias[tag] = p.addProxy(px)
	}
	for _, g := range groups {
		var members []string
		for _, m := range g.strs("outbounds") {
			if n, ok := alias[m]; ok {
				members = append(members, n)
			} else {
				members = append(members, m) // another group
			}
		}
		if len(members) == 0 {
			members = []string{"DIRECT"}
		}
		og := NewOMap().Set("name", g.s("tag"))
		if g.s("type") == "selector" {
			og.Set("type", "select")
			if def := g.s("default"); def != "" {
				if n, ok := alias[def]; ok {
					def = n
				}
				// mihomo selects the first member by default
				members = append([]string{def}, members...)
			}
			og.Set("proxies", uniqueStrings(members))
		} else {
			og.Set("type", "url-test").Set("proxies", uniqueStrings(members))
			og.Set("url", orDefault(g.s("url"), p.TestURL))
			og.Set("interval", durationSeconds(g.s("interval"), 180))
			if tol := g.i("tolerance"); tol > 0 {
				og.Set("tolerance", tol)
			}
			og.Set("lazy", true)
		}
		p.Groups = append(p.Groups, og)
	}
	if len(root.Route) > 0 {
		p.warn("sing-box route rules are not converted; the default rule template is used")
	}
	return p, nil
}

func durationSeconds(s string, def int) int {
	if s == "" {
		return def
	}
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return int(d.Seconds())
	}
	return def
}

func singBoxTLS(px *OMap, ob sbObj, sniKey string) {
	t := ob.o("tls")
	if !t.b("enabled") {
		return
	}
	if sniKey == "servername" {
		px.Set("tls", true)
	}
	px.SetStr(sniKey, t.s("server_name"))
	if t.b("insecure") {
		px.Set("skip-cert-verify", true)
	}
	if a := t.strs("alpn"); len(a) > 0 {
		px.Set("alpn", a)
	}
	if fp := t.o("utls").s("fingerprint"); fp != "" && t.o("utls").b("enabled") {
		px.Set("client-fingerprint", fp)
	}
	if r := t.o("reality"); r.b("enabled") {
		ro := NewOMap().Set("public-key", r.s("public_key"))
		ro.SetStr("short-id", r.s("short_id"))
		px.Set("reality-opts", ro)
		if px.GetString("client-fingerprint") == "" {
			px.Set("client-fingerprint", "chrome")
		}
	}
}

func singBoxTransport(px *OMap, ob sbObj) {
	t := ob.o("transport")
	host := ""
	if h := t.o("headers"); h != nil {
		host = sbObj(h).s("Host")
	}
	switch t.s("type") {
	case "ws":
		applyTransport(px, "ws", "", host, t.s("path"), "")
	case "httpupgrade":
		applyTransport(px, "httpupgrade", "", orDefault(t.s("host"), host), t.s("path"), "")
	case "grpc":
		applyTransport(px, "grpc", "", "", "", t.s("service_name"))
	case "http":
		applyTransport(px, "h2", "", strings.Join(t.strs("host"), ","), t.s("path"), "")
	}
}

func singBoxProxy(ob sbObj) (*OMap, error) {
	px := NewOMap().Set("name", ob.s("tag"))
	server, port := ob.s("server"), ob.i("server_port")
	base := func(t string) error {
		px.Set("type", t)
		if server == "" || port <= 0 {
			return fmt.Errorf("missing server or port")
		}
		px.Set("server", server).Set("port", port)
		return nil
	}
	switch ob.s("type") {
	case "shadowsocks":
		if err := base("ss"); err != nil {
			return nil, err
		}
		px.Set("cipher", ob.s("method")).Set("password", ob.s("password")).Set("udp", true)
		if pl := ob.s("plugin"); pl == "obfs-local" {
			opts := NewOMap()
			for _, kv := range strings.Split(ob.s("plugin_opts"), ";") {
				k, v, _ := strings.Cut(kv, "=")
				switch k {
				case "obfs":
					opts.Set("mode", v)
				case "obfs-host":
					opts.Set("host", v)
				}
			}
			px.Set("plugin", "obfs").Set("plugin-opts", opts)
		} else if pl != "" {
			return nil, fmt.Errorf("unsupported plugin %q", pl)
		}
	case "vmess":
		if err := base("vmess"); err != nil {
			return nil, err
		}
		px.Set("uuid", ob.s("uuid")).Set("alterId", ob.i("alter_id")).
			Set("cipher", orDefault(ob.s("security"), "auto")).Set("udp", true)
		singBoxTLS(px, ob, "servername")
		singBoxTransport(px, ob)
	case "vless":
		if err := base("vless"); err != nil {
			return nil, err
		}
		px.Set("uuid", ob.s("uuid")).Set("udp", true)
		px.SetStr("flow", ob.s("flow"))
		singBoxTLS(px, ob, "servername")
		singBoxTransport(px, ob)
	case "trojan":
		if err := base("trojan"); err != nil {
			return nil, err
		}
		px.Set("password", ob.s("password")).Set("udp", true)
		singBoxTLS(px, ob, "sni")
		singBoxTransport(px, ob)
	case "hysteria2":
		if err := base("hysteria2"); err != nil {
			return nil, err
		}
		px.Set("password", ob.s("password"))
		if o := ob.o("obfs"); o.s("type") == "salamander" {
			px.Set("obfs", "salamander").Set("obfs-password", o.s("password"))
		}
		if up := ob.i("up_mbps"); up > 0 {
			px.Set("up", fmt.Sprintf("%d Mbps", up))
		}
		if down := ob.i("down_mbps"); down > 0 {
			px.Set("down", fmt.Sprintf("%d Mbps", down))
		}
		if sp := ob.strs("server_ports"); len(sp) > 0 {
			px.Set("ports", strings.ReplaceAll(strings.Join(sp, ","), ":", "-"))
		}
		singBoxTLS(px, ob, "sni")
	case "hysteria":
		if err := base("hysteria"); err != nil {
			return nil, err
		}
		px.SetStr("auth-str", ob.s("auth_str"))
		px.Set("up", fmt.Sprintf("%d Mbps", max(ob.i("up_mbps"), 10)))
		px.Set("down", fmt.Sprintf("%d Mbps", max(ob.i("down_mbps"), 50)))
		px.SetStr("obfs", ob.s("obfs"))
		singBoxTLS(px, ob, "sni")
	case "tuic":
		if err := base("tuic"); err != nil {
			return nil, err
		}
		px.Set("uuid", ob.s("uuid")).Set("password", ob.s("password"))
		px.SetStr("congestion-controller", ob.s("congestion_control"))
		px.Set("udp-relay-mode", orDefault(ob.s("udp_relay_mode"), "native"))
		singBoxTLS(px, ob, "sni")
		if _, ok := px.Get("alpn"); !ok {
			px.Set("alpn", []string{"h3"})
		}
	case "anytls":
		if err := base("anytls"); err != nil {
			return nil, err
		}
		px.Set("password", ob.s("password")).Set("udp", true)
		singBoxTLS(px, ob, "sni")
	case "socks":
		if err := base("socks5"); err != nil {
			return nil, err
		}
		px.SetStr("username", ob.s("username"))
		px.SetStr("password", ob.s("password"))
		px.Set("udp", true)
	case "http":
		if err := base("http"); err != nil {
			return nil, err
		}
		px.SetStr("username", ob.s("username"))
		px.SetStr("password", ob.s("password"))
		if ob.o("tls").b("enabled") {
			px.Set("tls", true)
			singBoxTLS(px, ob, "sni")
		}
	case "wireguard":
		px.Set("type", "wireguard")
		peer := sbObj{}
		if peers, ok := ob["peers"].([]any); ok && len(peers) > 0 {
			peer, _ = peers[0].(map[string]any)
			server, port = peer.s("address"), peer.i("port")
		} else {
			peer = ob
			peer["public_key"] = ob.s("peer_public_key")
		}
		if server == "" || port <= 0 {
			return nil, fmt.Errorf("missing server or port")
		}
		px.Set("server", server).Set("port", port)
		px.Set("private-key", ob.s("private_key"))
		addrs := ob.strs("local_address")
		if len(addrs) == 0 {
			addrs = ob.strs("address")
		}
		for _, a := range addrs {
			ip := strings.SplitN(a, "/", 2)[0]
			if strings.Contains(ip, ":") {
				px.Set("ipv6", ip)
			} else {
				px.Set("ip", ip)
			}
		}
		px.Set("public-key", peer.s("public_key"))
		px.SetStr("pre-shared-key", peer.s("pre_shared_key"))
		if r, ok := peer["reserved"].([]any); ok && len(r) == 3 {
			var rr []int
			for _, x := range r {
				f, _ := x.(float64)
				rr = append(rr, int(f))
			}
			px.Set("reserved", rr)
		}
		if mtu := ob.i("mtu"); mtu > 0 {
			px.Set("mtu", mtu)
		}
		px.Set("udp", true)
	default:
		return nil, fmt.Errorf("unsupported type %q", ob.s("type"))
	}
	return px, nil
}
