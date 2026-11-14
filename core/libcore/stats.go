// SPDX-License-Identifier: GPL-3.0-or-later

package libcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/metacubex/mihomo/adapter/outboundgroup"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"
	"github.com/metacubex/mihomo/tunnel"
	"github.com/metacubex/mihomo/tunnel/statistic"
)

// Traffic holds byte counters. Up/Down are bytes per second.
type Traffic struct {
	Up        int64
	Down      int64
	UpTotal   int64
	DownTotal int64
}

// QueryTraffic returns the current speed and totals since start.
func QueryTraffic() *Traffic {
	up, down := statistic.DefaultManager.Now()
	ut, dt := statistic.DefaultManager.Total()
	return &Traffic{Up: up, Down: down, UpTotal: ut, DownTotal: dt}
}

// ResetTraffic zeroes the counters.
func ResetTraffic() { statistic.DefaultManager.ResetStatistic() }

type proxyItem struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Delay int    `json:"delay"` // ms; 0 = untested; -1 = timeout/failed
	UDP   bool   `json:"udp"`
}

type groupItem struct {
	Name    string      `json:"name"`
	Type    string      `json:"type"`
	Now     string      `json:"now"`
	Hidden  bool        `json:"hidden"`
	Icon    string      `json:"icon"`
	TestURL string      `json:"testUrl"`
	Proxies []proxyItem `json:"proxies"`
}

func delayOf(p C.Proxy, url string) int {
	d := p.LastDelayForTestUrl(url)
	if d != 0xffff {
		return int(d)
	}
	// distinguish "never tested" (0) from "tested and failed" (-1)
	if st, ok := p.ExtraDelayHistories()[url]; ok && len(st.History) > 0 {
		return -1
	}
	if url == "" && len(p.DelayHistory()) > 0 {
		return -1
	}
	return 0
}

// ProxyGroups returns the policy groups in profile order as JSON. When
// includeGlobal is true the built-in GLOBAL selector is listed first.
func ProxyGroups(includeGlobal bool) string {
	mu.Lock()
	order := append([]string(nil), groupOrder...)
	urls := map[string]string{}
	for k, v := range groupURL {
		urls[k] = v
	}
	def := "http://www.gstatic.com/generate_204"
	if current != nil && current.TestURL != "" {
		def = current.TestURL
	}
	mu.Unlock()

	proxies := tunnel.Proxies()
	if includeGlobal {
		order = append([]string{"GLOBAL"}, order...)
	}
	out := make([]groupItem, 0, len(order))
	for _, name := range order {
		p, ok := proxies[name]
		if !ok {
			continue
		}
		g, ok := p.Adapter().(outboundgroup.ProxyGroup)
		if !ok {
			continue
		}
		url := urls[name]
		if url == "" {
			url = def
		}
		item := groupItem{Name: name, Type: p.Type().String(), Now: g.Now(), Hidden: g.Hidden(), Icon: g.Icon(), TestURL: url}
		for _, m := range g.Proxies() {
			item.Proxies = append(item.Proxies, proxyItem{Name: m.Name(), Type: m.Type().String(), Delay: delayOf(m, url), UDP: m.SupportUDP()})
		}
		out = append(out, item)
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// SelectProxy selects member `name` in a select group.
func SelectProxy(group, name string) error {
	p, ok := tunnel.Proxies()[group]
	if !ok {
		return fmt.Errorf("group %q not found", group)
	}
	s, ok := p.Adapter().(outboundgroup.SelectAble)
	if !ok {
		return fmt.Errorf("group %q is not selectable", group)
	}
	if err := s.Set(name); err != nil {
		return err
	}
	// selections of select groups are persisted by mihomo's cachefile when
	// profile.store-selected is on (forced by patchRaw)
	return nil
}

// TestDelay measures one proxy's delay in ms (-1 on failure).
func TestDelay(name, url string, timeoutMs int32) int32 {
	p, ok := tunnel.Proxies()[name]
	if !ok {
		return -1
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()
	d, err := p.URLTest(ctx, url, nil)
	if err != nil {
		return -1
	}
	return int32(d)
}

// TestGroupDelay tests every member of a group concurrently and returns a
// JSON object {name: delayMs} (-1 for failures).
func TestGroupDelay(group, url string, timeoutMs int32) (string, error) {
	p, ok := tunnel.Proxies()[group]
	if !ok {
		return "", fmt.Errorf("group %q not found", group)
	}
	g, ok := p.Adapter().(outboundgroup.ProxyGroup)
	if !ok {
		return "", errors.New("not a group")
	}
	if url == "" {
		mu.Lock()
		url = groupURL[group]
		mu.Unlock()
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()
	res := map[string]int{}
	var lock sync.Mutex
	var wg sync.WaitGroup
	for _, m := range g.Proxies() {
		m := m
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := m.URLTest(ctx, url, nil)
			lock.Lock()
			if err != nil {
				res[m.Name()] = -1
			} else {
				res[m.Name()] = int(d)
			}
			lock.Unlock()
		}()
	}
	wg.Wait()
	b, _ := json.Marshal(res)
	return string(b), nil
}

type connItem struct {
	ID       string   `json:"id"`
	Network  string   `json:"network"`
	Host     string   `json:"host"`
	Dst      string   `json:"dst"`
	Process  string   `json:"process"`
	Rule     string   `json:"rule"`
	Payload  string   `json:"rulePayload"`
	Chains   []string `json:"chains"`
	Upload   int64    `json:"upload"`
	Download int64    `json:"download"`
	Start    int64    `json:"start"` // unix ms
}

// Connections returns active connections (newest first) as JSON.
func Connections() string {
	snap := statistic.DefaultManager.Snapshot()
	out := make([]connItem, 0, len(snap.Connections))
	for _, c := range snap.Connections {
		m := c.Metadata
		host := m.Host
		if host == "" {
			host = m.DstIP.String()
		}
		out = append(out, connItem{
			ID:       c.UUID.String(),
			Network:  m.NetWork.String(),
			Host:     fmt.Sprintf("%s:%d", host, m.DstPort),
			Dst:      m.RemoteAddress(),
			Process:  m.Process,
			Rule:     c.Rule,
			Payload:  c.RulePayload,
			Chains:   []string(c.Chain),
			Upload:   c.UploadTotal.Load(),
			Download: c.DownloadTotal.Load(),
			Start:    c.Start.UnixMilli(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start > out[j].Start })
	b, _ := json.Marshal(out)
	return string(b)
}

// CloseConnection closes one connection by id.
func CloseConnection(id string) bool {
	if c := statistic.DefaultManager.Get(id); c != nil {
		_ = c.Close()
		return true
	}
	return false
}

// CloseAllConnections closes every tracked connection.
func CloseAllConnections() {
	statistic.DefaultManager.Range(func(c statistic.Tracker) bool {
		_ = c.Close()
		return true
	})
}

// ---------------------------------------------------------------- logs

type logForwarder struct {
	stop chan struct{}
}

func startLogForwarder() {
	if logSub != nil {
		return
	}
	sub := log.Subscribe()
	f := &logForwarder{stop: make(chan struct{})}
	logSub = f
	// Drain mihomo's subscription without ever blocking: lines go into a
	// bounded queue and are dropped when the host is slow. Blocking here
	// would stall every log call inside mihomo (and with it the core).
	queue := make(chan log.Event, 1024)
	go func() {
		defer log.UnSubscribe(sub)
		for {
			select {
			case <-f.stop:
				close(queue)
				return
			case ev, ok := <-sub:
				if !ok {
					close(queue)
					return
				}
				if ev.LogLevel < log.Level() {
					continue
				}
				select {
				case queue <- ev:
				default: // host is behind; drop the line
				}
			}
		}
	}()
	go func() {
		for ev := range queue {
			b, _ := logPlatform.Load().(platformBox)
			if b.p != nil {
				b.p.OnLog(ev.Type(), strings.TrimSpace(ev.Payload))
			}
		}
	}()
}

// StopLogForwarder detaches the log stream (e.g. when the service dies).
func StopLogForwarder() {
	if logSub != nil {
		close(logSub.stop)
		logSub = nil
	}
}
