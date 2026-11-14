// SPDX-License-Identifier: GPL-3.0-or-later

package libcore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/metacubex/mihomo/log"
)

type fakePlatform struct {
	mu   sync.Mutex
	logs []string
}

func (f *fakePlatform) Protect(fd int32) bool { return true }
func (f *fakePlatform) FindPackage(network, srcIP string, srcPort int32, dstIP string, dstPort int32) string {
	return ""
}
func (f *fakePlatform) OnLog(level, msg string) {
	f.mu.Lock()
	f.logs = append(f.logs, level+": "+msg)
	f.mu.Unlock()
}

const surgeSample = `[General]
ipv6 = false
[Proxy]
A = ss, 127.0.0.1, 18388, encrypt-method=aes-128-gcm, password=x
B = trojan, 127.0.0.1, 18443, password=y, sni=example.com
[Proxy Group]
Proxy = select, A, B, Auto
Auto = url-test, A, B, interval=600
[Rule]
DOMAIN-SUFFIX,example.com,Proxy
FINAL,DIRECT
`

// TestLifecycle runs the core without a TUN (fd=0) on the host: convert ->
// start -> groups -> select -> traffic -> mode -> reload -> stop.
func TestLifecycle(t *testing.T) {
	home := t.TempDir()
	res, err := ConvertProfile([]byte(surgeSample), "", false)
	if err != nil {
		t.Fatal(err)
	}
	var conv struct {
		YAML   string `json:"yaml"`
		Format string `json:"format"`
	}
	if err := json.Unmarshal([]byte(res), &conv); err != nil || conv.Format != "surge" {
		t.Fatalf("bad convert result: %v %s", err, res)
	}
	cfgPath := filepath.Join(home, "config.yaml")
	// disable the mixed port so the test does not bind 7890
	yml := strings.Replace(conv.YAML, "mixed-port: 7890", "mixed-port: 0", 1)
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := TunInet4(cfgPath); got != "198.18.0.1/30" {
		t.Errorf("TunInet4 = %s", got)
	}

	fp := &fakePlatform{}
	SetPlatform(fp)
	if err := Validate(cfgPath, home); err != nil {
		t.Fatalf("validate: %v", err)
	}
	opt := NewStartOptions()
	opt.TunFd = 0
	opt.Mode = "rule"
	if err := Start(cfgPath, home, opt); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer Stop()
	if !IsRunning() {
		t.Fatal("not running")
	}

	var groups []struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		Now     string `json:"now"`
		Proxies []struct {
			Name  string `json:"name"`
			Delay int    `json:"delay"`
		} `json:"proxies"`
	}
	if err := json.Unmarshal([]byte(ProxyGroups(false)), &groups); err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[0].Name != "Proxy" || groups[1].Type != "URLTest" {
		t.Fatalf("groups = %+v", groups)
	}
	if err := SelectProxy("Proxy", "B"); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal([]byte(ProxyGroups(true)), &groups)
	if groups[0].Name != "GLOBAL" || groups[1].Now != "B" {
		t.Errorf("select not applied: %+v", groups)
	}
	if err := SelectProxy("Proxy", "nope"); err == nil {
		t.Error("selecting unknown proxy should fail")
	}
	// unreachable local ports -> delay test fails fast
	if d := TestDelay("A", "http://www.gstatic.com/generate_204", 1500); d != -1 {
		t.Errorf("expected -1 delay for dead proxy, got %d", d)
	}
	if _, err := TestGroupDelay("Auto", "", 1500); err != nil {
		t.Error(err)
	}
	if tr := QueryTraffic(); tr.UpTotal < 0 {
		t.Error("negative traffic")
	}
	if err := SetMode("global"); err != nil || GetMode() != "global" {
		t.Errorf("mode switch failed: %v %s", err, GetMode())
	}
	if err := SetMode("bogus"); err == nil {
		t.Error("bogus mode accepted")
	}
	if err := Reload(cfgPath); err != nil {
		t.Errorf("reload: %v", err)
	}
	if !strings.HasPrefix(Connections(), "[") {
		t.Error("connections not json array")
	}
	time.Sleep(200 * time.Millisecond)
	fp.mu.Lock()
	nlogs := len(fp.logs)
	fp.mu.Unlock()
	if nlogs == 0 {
		t.Error("no logs forwarded")
	}
	Stop()
	if IsRunning() {
		t.Error("still running after Stop")
	}
	if !strings.Contains(Version(), "mihomo") {
		t.Error(Version())
	}
	if ui := ParseUserinfo("upload=1;download=2;total=3;expire=4"); !strings.Contains(ui, `"total":3`) {
		t.Error(ui)
	}
}

func TestConvertProfileWithTemplate(t *testing.T) {
	var list []map[string]any
	if err := json.Unmarshal([]byte(BuiltinTemplates()), &list); err != nil || len(list) < 2 {
		t.Fatalf("BuiltinTemplates: %v %d", err, len(list))
	}
	out, err := ConvertProfileWithTemplate([]byte(surgeSample), "", false, "builtin:simple", "replace")
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		YAML string `json:"yaml"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.YAML, "节点选择") || strings.Contains(res.YAML, "example.com,Proxy") {
		t.Errorf("template not applied:\n%s", res.YAML)
	}
	if msg := ValidateTemplate("rules:\n  - MATCH,DIRECT\n"); msg != "" {
		t.Errorf("valid template rejected: %s", msg)
	}
	if msg := ValidateTemplate("nonsense"); msg == "" {
		t.Error("invalid template accepted")
	}
}

// Regression: with mu held (as Start holds it while mihomo loads rule
// providers), a burst of core log lines must not block. Before the fix the
// forwarder waited on mu, mihomo's subscriber buffer (200) filled, and every
// log call in the core blocked: the VPN stayed on "connecting" forever.
func TestLogBurstWhileLocked(t *testing.T) {
	SetPlatform(&fakePlatform{})
	startLogForwarder()
	defer StopLogForwarder()

	mu.Lock()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 2000; i++ {
			log.Warnln("burst %d", i)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		mu.Unlock()
		t.Fatal("mihomo log calls blocked while mu was held")
	}
	mu.Unlock()
}
