// SPDX-License-Identifier: GPL-3.0-or-later

package convert

import (
	"encoding/json"

	"go.yaml.in/yaml/v3"
)

// OMap is an insertion-ordered string-keyed map. It marshals to YAML/JSON
// with keys in insertion order, which keeps the generated mihomo config
// readable (name/type/server/port first).
type OMap struct {
	keys []string
	vals map[string]any
}

func NewOMap() *OMap { return &OMap{vals: map[string]any{}} }

// Set inserts or replaces key. A nil value is ignored so callers can set
// optional fields unconditionally.
func (m *OMap) Set(key string, val any) *OMap {
	if val == nil {
		return m
	}
	if _, ok := m.vals[key]; !ok {
		m.keys = append(m.keys, key)
	}
	m.vals[key] = val
	return m
}

// SetIf sets key only when cond is true.
func (m *OMap) SetIf(cond bool, key string, val any) *OMap {
	if cond {
		m.Set(key, val)
	}
	return m
}

// SetStr sets key only when val is non-empty.
func (m *OMap) SetStr(key, val string) *OMap {
	if val != "" {
		m.Set(key, val)
	}
	return m
}

func (m *OMap) Get(key string) (any, bool) {
	v, ok := m.vals[key]
	return v, ok
}

func (m *OMap) GetString(key string) string {
	if v, ok := m.vals[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (m *OMap) Delete(key string) {
	if _, ok := m.vals[key]; !ok {
		return
	}
	delete(m.vals, key)
	for i, k := range m.keys {
		if k == key {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			break
		}
	}
}

func (m *OMap) Keys() []string { return append([]string(nil), m.keys...) }

func (m *OMap) Len() int { return len(m.keys) }

// Map returns a plain map copy (nested OMaps are converted recursively).
func (m *OMap) Map() map[string]any {
	out := make(map[string]any, len(m.keys))
	for _, k := range m.keys {
		out[k] = plain(m.vals[k])
	}
	return out
}

func plain(v any) any {
	switch t := v.(type) {
	case *OMap:
		return t.Map()
	case []*OMap:
		arr := make([]any, len(t))
		for i, x := range t {
			arr[i] = x.Map()
		}
		return arr
	case []any:
		arr := make([]any, len(t))
		for i, x := range t {
			arr[i] = plain(x)
		}
		return arr
	default:
		return v
	}
}

func (m *OMap) MarshalYAML() (any, error) {
	node := &yaml.Node{Kind: yaml.MappingNode}
	for _, k := range m.keys {
		kn := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}
		vn := &yaml.Node{}
		if err := vn.Encode(m.vals[k]); err != nil {
			return nil, err
		}
		node.Content = append(node.Content, kn, vn)
	}
	return node, nil
}

func (m *OMap) MarshalJSON() ([]byte, error) {
	buf := []byte{'{'}
	for i, k := range m.keys {
		if i > 0 {
			buf = append(buf, ',')
		}
		kb, _ := json.Marshal(k)
		vb, err := json.Marshal(m.vals[k])
		if err != nil {
			return nil, err
		}
		buf = append(buf, kb...)
		buf = append(buf, ':')
		buf = append(buf, vb...)
	}
	return append(buf, '}'), nil
}
