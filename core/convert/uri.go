// SPDX-License-Identifier: GPL-3.0-or-later

package convert

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// ParseURIList parses share links, one per line. Unsupported schemes are
// reported through warn and skipped.
func ParseURIList(text string, warn func(string, ...any)) []*OMap {
	var out []*OMap
	for n, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || isComment(line) {
			continue
		}
		i := strings.Index(line, "://")
		if i <= 0 {
			continue
		}
		scheme := strings.ToLower(line[:i])
		var (
			px  *OMap
			err error
		)
		switch scheme {
		case "ss":
			px, err = parseSS(line)
		case "ssr":
			err = fmt.Errorf("ShadowsocksR links are not supported")
		case "vmess":
			px, err = parseVmess(line)
		case "vless":
			px, err = parseVless(line)
		case "trojan":
			px, err = parseTrojan(line)
		case "hysteria2", "hy2":
			px, err = parseHysteria2(line)
		case "hysteria":
			px, err = parseHysteria(line)
		case "tuic":
			px, err = parseTuic(line)
		case "anytls":
			px, err = parseAnyTLS(line)
		case "socks", "socks5", "http", "https":
			px, err = parseSocksHTTP(scheme, line)
		default:
			err = fmt.Errorf("unsupported scheme %q", scheme)
		}
		if err != nil {
			warn("link %d skipped: %v", n+1, err)
			continue
		}
		out = append(out, px)
	}
	return out
}

func b64decode(s string) (string, error) {
	s = strings.TrimSpace(s)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("invalid base64")
}

func fragmentName(u *url.URL, fallback string) string {
	if u.Fragment != "" {
		return u.Fragment
	}
	return fallback
}

func portOf(u *url.URL) (int, error) {
	p, err := strconv.Atoi(u.Port())
	if err != nil || p <= 0 || p > 65535 {
		return 0, fmt.Errorf("invalid port")
	}
	return p, nil
}

func insecure(q url.Values) bool {
	for _, k := range []string{"allowInsecure", "insecure", "allow_insecure", "skip-cert-verify"} {
		if parseBool(q.Get(k)) {
			return true
		}
	}
	return false
}

func parseSS(link string) (*OMap, error) {
	body := strings.TrimPrefix(link, link[:strings.Index(link, "://")+3])
	name := ""
	if i := strings.Index(body, "#"); i >= 0 {
		name, _ = url.PathUnescape(body[i+1:])
		body = body[:i]
	}
	// Legacy form: ss://BASE64(method:password@host:port)
	if !strings.Contains(body, "@") {
		q := ""
		if i := strings.IndexAny(body, "?/"); i >= 0 {
			body, q = body[:i], body[i:]
		}
		dec, err := b64decode(body)
		if err != nil {
			return nil, err
		}
		body = dec + q
	}
	u, err := url.Parse("ss://" + body)
	if err != nil {
		return nil, err
	}
	var method, password string
	if u.User != nil {
		if pw, ok := u.User.Password(); ok {
			method, password = u.User.Username(), pw
		} else if dec, err := b64decode(u.User.Username()); err == nil && strings.Contains(dec, ":") {
			parts := strings.SplitN(dec, ":", 2)
			method, password = parts[0], parts[1]
		} else {
			return nil, fmt.Errorf("invalid ss userinfo")
		}
	}
	port, err := portOf(u)
	if err != nil {
		return nil, err
	}
	px := NewOMap().Set("name", orDefault(name, u.Hostname())).Set("type", "ss").
		Set("server", u.Hostname()).Set("port", port).Set("cipher", method).Set("password", password).Set("udp", true)
	if plugin := u.Query().Get("plugin"); plugin != "" {
		parts := strings.Split(plugin, ";")
		opts := NewOMap()
		switch parts[0] {
		case "obfs-local", "simple-obfs":
			px.Set("plugin", "obfs")
			for _, kv := range parts[1:] {
				k, v, _ := strings.Cut(kv, "=")
				switch k {
				case "obfs":
					opts.Set("mode", v)
				case "obfs-host":
					opts.Set("host", v)
				}
			}
		case "v2ray-plugin":
			px.Set("plugin", "v2ray-plugin")
			opts.Set("mode", "websocket")
			for _, kv := range parts[1:] {
				k, v, _ := strings.Cut(kv, "=")
				switch k {
				case "mode":
					opts.Set("mode", v)
				case "tls":
					opts.Set("tls", true)
				case "host":
					opts.Set("host", v)
				case "path":
					opts.Set("path", v)
				case "mux":
					opts.Set("mux", parseBool(v))
				}
			}
		default:
			return nil, fmt.Errorf("unsupported ss plugin %q", parts[0])
		}
		px.Set("plugin-opts", opts)
	}
	return px, nil
}

func parseVmess(link string) (*OMap, error) {
	dec, err := b64decode(strings.TrimPrefix(link, link[:strings.Index(link, "://")+3]))
	if err != nil {
		return nil, fmt.Errorf("vmess: %w", err)
	}
	var j map[string]any
	if err := json.Unmarshal([]byte(dec), &j); err != nil {
		return nil, fmt.Errorf("vmess: invalid json")
	}
	str := func(k string) string {
		switch v := j[k].(type) {
		case string:
			return v
		case float64:
			return strconv.Itoa(int(v))
		}
		return ""
	}
	port := atoi(str("port"))
	if str("add") == "" || port <= 0 {
		return nil, fmt.Errorf("vmess: missing address/port")
	}
	px := NewOMap().Set("name", orDefault(str("ps"), str("add"))).Set("type", "vmess").
		Set("server", str("add")).Set("port", port).Set("uuid", str("id")).
		Set("alterId", atoi(str("aid"))).Set("cipher", orDefault(str("scy"), "auto")).Set("udp", true)
	if str("tls") == "tls" {
		px.Set("tls", true)
		px.SetStr("servername", orDefault(str("sni"), str("host")))
		if a := splitList(str("alpn")); len(a) > 0 {
			px.Set("alpn", a)
		}
		px.SetStr("client-fingerprint", str("fp"))
	}
	applyTransport(px, str("net"), str("type"), str("host"), str("path"), str("path"))
	return px, nil
}

// applyTransport maps v2ray-style transport parameters.
func applyTransport(px *OMap, network, headerType, host, path, serviceName string) {
	switch network {
	case "ws":
		px.Set("network", "ws")
		o := NewOMap().Set("path", orDefault(path, "/"))
		if host != "" {
			o.Set("headers", NewOMap().Set("Host", host))
		}
		px.Set("ws-opts", o)
	case "httpupgrade":
		px.Set("network", "ws")
		o := NewOMap().Set("path", orDefault(path, "/")).Set("v2ray-http-upgrade", true)
		if host != "" {
			o.Set("headers", NewOMap().Set("Host", host))
		}
		px.Set("ws-opts", o)
	case "grpc":
		px.Set("network", "grpc")
		px.Set("grpc-opts", NewOMap().Set("grpc-service-name", serviceName))
	case "h2":
		px.Set("network", "h2")
		o := NewOMap().Set("path", orDefault(path, "/"))
		if host != "" {
			o.Set("host", splitList(host))
		}
		px.Set("h2-opts", o)
	case "xhttp", "splithttp":
		px.Set("network", "xhttp")
		o := NewOMap().Set("path", orDefault(path, "/"))
		o.SetStr("host", host)
		px.Set("xhttp-opts", o)
	case "tcp", "":
		if headerType == "http" {
			px.Set("network", "http")
			o := NewOMap().Set("path", []string{orDefault(path, "/")})
			if host != "" {
				o.Set("headers", NewOMap().Set("Host", splitList(host)))
			}
			px.Set("http-opts", o)
		}
	}
}

func parseVless(link string) (*OMap, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, err
	}
	port, err := portOf(u)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	px := NewOMap().Set("name", fragmentName(u, u.Hostname())).Set("type", "vless").
		Set("server", u.Hostname()).Set("port", port).Set("uuid", u.User.Username()).Set("udp", true)
	px.SetStr("flow", q.Get("flow"))
	switch q.Get("security") {
	case "tls", "reality":
		px.Set("tls", true)
		px.SetStr("servername", q.Get("sni"))
		if a := splitList(q.Get("alpn")); len(a) > 0 {
			px.Set("alpn", a)
		}
		fp := q.Get("fp")
		if q.Get("security") == "reality" {
			ro := NewOMap().Set("public-key", q.Get("pbk"))
			ro.SetStr("short-id", q.Get("sid"))
			px.Set("reality-opts", ro)
			fp = orDefault(fp, "chrome")
		}
		px.SetStr("client-fingerprint", fp)
	}
	if insecure(q) {
		px.Set("skip-cert-verify", true)
	}
	applyTransport(px, q.Get("type"), q.Get("headerType"), q.Get("host"), q.Get("path"), q.Get("serviceName"))
	if px.GetString("network") == "xhttp" {
		if mode := q.Get("mode"); mode != "" {
			o, _ := px.Get("xhttp-opts")
			o.(*OMap).Set("mode", mode)
		}
	}
	return px, nil
}

func parseTrojan(link string) (*OMap, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, err
	}
	port, err := portOf(u)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	px := NewOMap().Set("name", fragmentName(u, u.Hostname())).Set("type", "trojan").
		Set("server", u.Hostname()).Set("port", port).Set("password", u.User.Username()).Set("udp", true)
	px.SetStr("sni", orDefault(q.Get("sni"), q.Get("peer")))
	if a := splitList(q.Get("alpn")); len(a) > 0 {
		px.Set("alpn", a)
	}
	px.SetStr("client-fingerprint", q.Get("fp"))
	if insecure(q) {
		px.Set("skip-cert-verify", true)
	}
	applyTransport(px, q.Get("type"), q.Get("headerType"), q.Get("host"), q.Get("path"), q.Get("serviceName"))
	return px, nil
}

var hy2MultiPortRe = regexp.MustCompile(`^([a-z0-9]+://(?:[^@/?#]*@)?(?:\[[^\]]+\]|[^:/?#]+)):(\d+[-,][0-9,-]+)`)

func parseHysteria2(link string) (*OMap, error) {
	ports := ""
	// "host:20000-30000" multi-port authority is not a valid URL port
	if m := hy2MultiPortRe.FindStringSubmatch(link); m != nil {
		ports = m[2]
		first := strings.FieldsFunc(ports, func(r rune) bool { return r == '-' || r == ',' })[0]
		link = m[1] + ":" + first + link[len(m[0]):]
	}
	u, err := url.Parse(link)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	port := atoi(u.Port())
	if mp := q.Get("mport"); mp != "" {
		ports = mp
	}
	if port <= 0 {
		port = 443
	}
	password := ""
	if u.User != nil {
		password = u.User.String()
		if dec, err := url.PathUnescape(password); err == nil {
			password = dec
		}
	}
	px := NewOMap().Set("name", fragmentName(u, u.Hostname())).Set("type", "hysteria2").
		Set("server", u.Hostname()).Set("port", port).Set("password", password)
	px.SetStr("ports", ports)
	px.SetStr("sni", q.Get("sni"))
	if q.Get("obfs") == "salamander" {
		px.Set("obfs", "salamander").Set("obfs-password", q.Get("obfs-password"))
	}
	if insecure(q) {
		px.Set("skip-cert-verify", true)
	}
	px.SetStr("fingerprint", q.Get("pinSHA256"))
	return px, nil
}

func parseHysteria(link string) (*OMap, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, err
	}
	port, err := portOf(u)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	px := NewOMap().Set("name", fragmentName(u, u.Hostname())).Set("type", "hysteria").
		Set("server", u.Hostname()).Set("port", port)
	px.SetStr("auth-str", q.Get("auth"))
	px.SetStr("protocol", q.Get("protocol"))
	px.SetStr("sni", q.Get("peer"))
	px.Set("up", orDefault(q.Get("upmbps"), "50")+" Mbps")
	px.Set("down", orDefault(q.Get("downmbps"), "100")+" Mbps")
	if a := splitList(q.Get("alpn")); len(a) > 0 {
		px.Set("alpn", a)
	}
	px.SetStr("obfs", q.Get("obfsParam"))
	if insecure(q) {
		px.Set("skip-cert-verify", true)
	}
	return px, nil
}

func parseTuic(link string) (*OMap, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, err
	}
	port, err := portOf(u)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	pw, _ := u.User.Password()
	px := NewOMap().Set("name", fragmentName(u, u.Hostname())).Set("type", "tuic").
		Set("server", u.Hostname()).Set("port", port).Set("uuid", u.User.Username()).Set("password", pw)
	if a := splitList(q.Get("alpn")); len(a) > 0 {
		px.Set("alpn", a)
	} else {
		px.Set("alpn", []string{"h3"})
	}
	px.SetStr("sni", q.Get("sni"))
	px.SetStr("congestion-controller", q.Get("congestion_control"))
	px.Set("udp-relay-mode", orDefault(q.Get("udp_relay_mode"), "native"))
	if parseBool(q.Get("disable_sni")) {
		px.Set("disable-sni", true)
	}
	if insecure(q) {
		px.Set("skip-cert-verify", true)
	}
	return px, nil
}

func parseAnyTLS(link string) (*OMap, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, err
	}
	port, err := portOf(u)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	px := NewOMap().Set("name", fragmentName(u, u.Hostname())).Set("type", "anytls").
		Set("server", u.Hostname()).Set("port", port).Set("password", u.User.Username()).Set("udp", true)
	px.SetStr("sni", q.Get("sni"))
	px.Set("client-fingerprint", orDefault(q.Get("fp"), "chrome"))
	if insecure(q) {
		px.Set("skip-cert-verify", true)
	}
	return px, nil
}

func parseSocksHTTP(scheme, link string) (*OMap, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, err
	}
	port, err := portOf(u)
	if err != nil {
		return nil, err
	}
	typ := "socks5"
	if scheme == "http" || scheme == "https" {
		typ = "http"
	}
	px := NewOMap().Set("name", fragmentName(u, u.Hostname())).Set("type", typ).
		Set("server", u.Hostname()).Set("port", port)
	if u.User != nil {
		user := u.User.Username()
		pw, hasPw := u.User.Password()
		if !hasPw {
			if dec, err := b64decode(user); err == nil && strings.Contains(dec, ":") {
				user, pw, _ = strings.Cut(dec, ":")
			}
		}
		px.SetStr("username", user)
		px.SetStr("password", pw)
	}
	if scheme == "https" {
		px.Set("tls", true)
	}
	if typ == "socks5" {
		px.Set("udp", true)
	}
	return px, nil
}
