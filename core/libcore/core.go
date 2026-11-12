// SPDX-License-Identifier: GPL-3.0-or-later

// Package libcore is the gomobile-bound bridge between the Android app and
// the mihomo core. Every exported identifier here becomes part of the Java
// API (io.github.sqmydor.surfmi.core.libcore.Libcore), so only gomobile
// compatible types are used: string, bool, int32/int64, []byte, error,
// pointers to structs with such fields, and interfaces.
//
// Design notes (independent implementation; ClashMetaForAndroid's GPL core
// was consulted for which mihomo hooks exist, no code was copied):
//   - built with the `cmfa` tag so mihomo delegates package-name lookup and
//     system DNS to the host app instead of reading /proc or packages.list;
//   - the TUN device is created by Android's VpnService and its fd is handed
//     to mihomo's sing-tun listener via tun.file-descriptor;
//   - every outbound socket mihomo opens is passed to VpnService.protect()
//     through dialer.DefaultSocketHook so it bypasses the VPN.
package libcore

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"sync"
	"syscall"

	"github.com/metacubex/mihomo/component/dialer"
	"github.com/metacubex/mihomo/component/process"
	"github.com/metacubex/mihomo/config"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/hub/executor"
	"github.com/metacubex/mihomo/log"
	"github.com/metacubex/mihomo/tunnel"
	"github.com/metacubex/mihomo/tunnel/statistic"
)

// WrapperVersion is the version of this bridge API.
const WrapperVersion = "0.1.0"

// Platform is implemented on the Kotlin side by the VpnService.
type Platform interface {
	// Protect marks a socket so it bypasses the VPN. Returns false on failure.
	Protect(fd int32) bool
	// FindPackage returns the package name owning a connection (via
	// ConnectivityManager.getConnectionOwnerUid) or "" when unknown.
	FindPackage(network string, srcIP string, srcPort int32, dstIP string, dstPort int32) string
	// OnLog receives core log lines.
	OnLog(level string, message string)
}

// StartOptions are runtime overrides applied on top of the profile.
type StartOptions struct {
	TunFd    int32  // fd from VpnService.Builder.establish()
	Stack    string // gvisor | system | mixed
	MTU      int32
	IPv6     bool
	Mode     string // rule | global | direct ; "" keeps the profile's mode
	LogLevel string // debug | info | warning | error | silent
	TestURL  string // default latency test URL
	// DNSServers optionally replaces dns.nameserver (comma separated).
	DNSServers string
	// ExternalController optionally exposes the mihomo REST API, e.g. "127.0.0.1:9090".
	ExternalController string
	Secret             string
}

// NewStartOptions returns options with sensible defaults.
func NewStartOptions() *StartOptions {
	return &StartOptions{Stack: "gvisor", MTU: 9000, IPv6: false, LogLevel: "info", TestURL: "http://www.gstatic.com/generate_204"}
}

var (
	mu         sync.Mutex
	platform   Platform
	running    bool
	current    *StartOptions
	groupOrder []string
	groupURL   = map[string]string{}
	logSub     *logForwarder
)

// SetPlatform registers the host callbacks. Call before Start.
func SetPlatform(p Platform) {
	mu.Lock()
	defer mu.Unlock()
	platform = p
	installHooks()
}

func installHooks() {
	p := platform
	if p == nil {
		dialer.DefaultSocketHook = nil
		process.DefaultPackageNameResolver = nil
		return
	}
	dialer.DefaultSocketHook = func(network, address string, conn syscall.RawConn) error {
		var ok bool
		err := conn.Control(func(fd uintptr) { ok = p.Protect(int32(fd)) })
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("protect socket failed")
		}
		return nil
	}
	process.DefaultPackageNameResolver = func(m *C.Metadata) (string, error) {
		pkg := p.FindPackage(m.NetWork.String(), m.SrcIP.String(), int32(m.SrcPort), m.DstIP.String(), int32(m.DstPort))
		if pkg == "" {
			return "", process.ErrNotFound
		}
		return pkg, nil
	}
}

// Version reports the bundled mihomo version.
func Version() string {
	return fmt.Sprintf("mihomo %s (libcore %s)", C.Version, WrapperVersion)
}

// IsRunning reports whether the core is started.
func IsRunning() bool {
	mu.Lock()
	defer mu.Unlock()
	return running
}

// TunInet4 returns the IPv4 address/prefix mihomo will assign to the TUN for
// the given profile (derived from dns.fake-ip-range). The VpnService must
// configure the same address before handing over the fd.
func TunInet4(configPath string) string {
	buf, err := os.ReadFile(configPath)
	if err != nil {
		return "198.18.0.1/30"
	}
	raw, err := config.UnmarshalRawConfig(buf)
	if err != nil || raw.DNS.FakeIPRange == "" {
		return "198.18.0.1/30"
	}
	pfx, err := netip.ParsePrefix(raw.DNS.FakeIPRange)
	if err != nil || !pfx.Addr().Is4() {
		return "198.18.0.1/30"
	}
	return netip.PrefixFrom(pfx.Addr(), 30).String()
}

// TunInet6 is the IPv6 address used on the TUN when IPv6 is enabled.
const TunInet6 = "fdfe:dcba:9876::1/126"

func loadPatched(configPath, homeDir string, opt *StartOptions) (*config.Config, *rawInfo, error) {
	if homeDir != "" {
		C.SetHomeDir(homeDir)
	}
	C.SetConfig(configPath)
	buf, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, err
	}
	raw, err := config.UnmarshalRawConfig(buf)
	if err != nil {
		return nil, nil, err
	}
	patchRaw(raw, opt)
	// capture group order/urls before ParseRawConfig, which sorts groups
	// topologically in place
	info := &rawInfo{urls: map[string]string{}, stack: raw.Tun.Stack.String()}
	for _, g := range raw.ProxyGroup {
		name, _ := g["name"].(string)
		if name == "" {
			continue
		}
		info.order = append(info.order, name)
		if u, ok := g["url"].(string); ok && u != "" {
			info.urls[name] = u
		}
	}
	cfg, err := config.ParseRawConfig(raw)
	if err != nil {
		return nil, nil, err
	}
	return cfg, info, nil
}

type rawInfo struct {
	order []string
	urls  map[string]string
	stack string
}

// patchRaw forces the settings that must hold on Android regardless of the
// profile: TUN via fd, no auto-route (VpnService routes), DNS hijack, no
// listening ports beyond what the user asked for.
func patchRaw(raw *config.RawConfig, opt *StartOptions) {
	if opt == nil {
		opt = NewStartOptions()
	}
	raw.ExternalController = opt.ExternalController
	raw.Secret = opt.Secret
	raw.ExternalControllerTLS = ""
	raw.ExternalControllerUnix = ""
	raw.ExternalControllerPipe = ""
	raw.ExternalUI = ""
	raw.Interface = ""
	raw.RoutingMark = 0
	raw.Port, raw.SocksPort, raw.RedirPort, raw.TProxyPort = 0, 0, 0, 0
	raw.IPTables.Enable = false

	if opt.Mode != "" {
		if m, ok := tunnel.ModeMapping[strings.ToLower(opt.Mode)]; ok {
			raw.Mode = m
		}
	}
	if opt.LogLevel != "" {
		if l, ok := log.LogLevelMapping[strings.ToLower(opt.LogLevel)]; ok {
			raw.LogLevel = l
		}
	}
	raw.IPv6 = opt.IPv6
	raw.Profile.StoreSelected = true

	if !raw.DNS.Enable {
		raw.DNS.Enable = true
		raw.DNS.EnhancedMode = C.DNSFakeIP
		if raw.DNS.FakeIPRange == "" {
			raw.DNS.FakeIPRange = "198.18.0.1/16"
		}
		if len(raw.DNS.DefaultNameserver) == 0 {
			raw.DNS.DefaultNameserver = []string{"223.5.5.5", "119.29.29.29"}
		}
		if len(raw.DNS.NameServer) == 0 {
			raw.DNS.NameServer = []string{"https://dns.alidns.com/dns-query", "https://doh.pub/dns-query"}
		}
	}
	if s := strings.TrimSpace(opt.DNSServers); s != "" {
		var list []string
		for _, x := range strings.Split(s, ",") {
			if x = strings.TrimSpace(x); x != "" {
				list = append(list, x)
			}
		}
		if len(list) > 0 {
			raw.DNS.NameServer = list
		}
	}
	raw.DNS.IPv6 = opt.IPv6
	raw.DNS.Listen = ""

	raw.Tun.Enable = opt.TunFd > 0
	raw.Tun.FileDescriptor = int(opt.TunFd)
	raw.Tun.Device = ""
	raw.Tun.AutoRoute = false
	raw.Tun.AutoRedirect = false
	raw.Tun.AutoDetectInterface = false
	raw.Tun.StrictRoute = false
	raw.Tun.DNSHijack = []string{"0.0.0.0:53", "[::]:53"}
	if st, ok := C.StackTypeMapping[strings.ToLower(opt.Stack)]; ok {
		raw.Tun.Stack = st
	} else {
		raw.Tun.Stack = C.TunGvisor
	}
	if opt.MTU > 0 {
		raw.Tun.MTU = uint32(opt.MTU)
	}
	if opt.IPv6 {
		raw.Tun.Inet6Address = []netip.Prefix{netip.MustParsePrefix(TunInet6)}
	} else {
		raw.Tun.Inet6Address = nil
	}
}

func rememberGroups(info *rawInfo, testURL string) {
	groupOrder = append([]string(nil), info.order...)
	groupURL = map[string]string{}
	for _, name := range info.order {
		if u := info.urls[name]; u != "" {
			groupURL[name] = u
		} else {
			groupURL[name] = testURL
		}
	}
}

// Start parses the profile at configPath, applies Android overrides and
// starts the core. homeDir holds caches, providers and geo databases.
func Start(configPath, homeDir string, opt *StartOptions) error {
	mu.Lock()
	defer mu.Unlock()
	if opt == nil {
		opt = NewStartOptions()
	}
	installHooks()
	startLogForwarder()
	cfg, info, err := loadPatched(configPath, homeDir, opt)
	if err != nil {
		return fmt.Errorf("load profile: %w", err)
	}
	if running {
		executor.Shutdown()
	}
	executor.ApplyConfig(cfg, true)
	current = opt
	rememberGroups(info, opt.TestURL)
	running = true
	log.Infoln("[Surfmi] core started, tun fd=%d stack=%s", opt.TunFd, info.stack)
	return nil
}

// Reload re-reads the profile (e.g. after an update) keeping the TUN fd.
func Reload(configPath string) error {
	mu.Lock()
	defer mu.Unlock()
	if !running || current == nil {
		return errors.New("core is not running")
	}
	cfg, info, err := loadPatched(configPath, "", current)
	if err != nil {
		return fmt.Errorf("load profile: %w", err)
	}
	executor.ApplyConfig(cfg, false)
	rememberGroups(info, current.TestURL)
	log.Infoln("[Surfmi] profile reloaded")
	return nil
}

// Validate checks that mihomo accepts the profile without starting it.
func Validate(configPath, homeDir string) error {
	mu.Lock()
	defer mu.Unlock()
	opt := NewStartOptions()
	opt.TunFd = 0
	_, _, err := loadPatched(configPath, homeDir, opt)
	return err
}

// Stop shuts the core down and closes all connections.
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if !running {
		return
	}
	statistic.DefaultManager.Range(func(c statistic.Tracker) bool {
		_ = c.Close()
		return true
	})
	executor.Shutdown()
	running = false
	current = nil
	log.Infoln("[Surfmi] core stopped")
}

// SetMode switches rule/global/direct at runtime.
func SetMode(mode string) error {
	m, ok := tunnel.ModeMapping[strings.ToLower(mode)]
	if !ok {
		return fmt.Errorf("unknown mode %q", mode)
	}
	tunnel.SetMode(m)
	mu.Lock()
	if current != nil {
		current.Mode = mode
	}
	mu.Unlock()
	return nil
}

// GetMode returns the current tunnel mode.
func GetMode() string { return tunnel.Mode().String() }

// SetLogLevel changes the core log level at runtime.
func SetLogLevel(level string) {
	if l, ok := log.LogLevelMapping[strings.ToLower(level)]; ok {
		log.SetLevel(l)
	}
}
