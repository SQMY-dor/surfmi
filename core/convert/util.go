// SPDX-License-Identifier: GPL-3.0-or-later

package convert

import (
	"strconv"
	"strings"
)

func isComment(line string) bool {
	return strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//")
}

// splitTop splits s on sep at nesting depth 0, ignoring separators inside
// double quotes or parentheses. Each part is trimmed.
func splitTop(s string, sep byte) []string {
	var (
		parts []string
		depth int
		inQ   bool
		start int
	)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			inQ = !inQ
		case inQ:
		case c == '(':
			depth++
		case c == ')':
			if depth > 0 {
				depth--
			}
		case c == sep && depth == 0:
			parts = append(parts, strings.TrimSpace(s[start:i]))
			start = i + 1
		}
	}
	parts = append(parts, strings.TrimSpace(s[start:]))
	return parts
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// splitKV splits "key = value" at the first '=' (outside quotes).
func splitKV(s string) (string, string, bool) {
	inQ := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			inQ = !inQ
		case '=':
			if !inQ {
				return strings.TrimSpace(s[:i]), unquote(s[i+1:]), true
			}
		}
	}
	return strings.TrimSpace(s), "", false
}

// kvList is an ordered key=value collection with positional leftovers.
type kvList struct {
	pos  []string
	keys []string
	m    map[string]string
}

func parseKVTokens(tokens []string) *kvList {
	kv := &kvList{m: map[string]string{}}
	for _, t := range tokens {
		if t == "" {
			continue
		}
		if k, v, ok := splitKV(t); ok && isOptionKey(k) {
			lk := strings.ToLower(k)
			if _, dup := kv.m[lk]; !dup {
				kv.keys = append(kv.keys, lk)
			}
			kv.m[lk] = v
			continue
		}
		kv.pos = append(kv.pos, unquote(t))
	}
	return kv
}

// isOptionKey reports whether k looks like an option name (no spaces etc).
func isOptionKey(k string) bool {
	if k == "" {
		return false
	}
	for _, r := range k {
		if !(r == '-' || r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func (kv *kvList) get(k string) string { return kv.m[k] }

func (kv *kvList) has(k string) bool { _, ok := kv.m[k]; return ok }

func (kv *kvList) bool(k string) (bool, bool) {
	v, ok := kv.m[k]
	if !ok {
		return false, false
	}
	return parseBool(v), true
}

func (kv *kvList) int(k string) (int, bool) {
	v, ok := kv.m[k]
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, false
	}
	return n, true
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on":
		return true
	}
	return false
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func splitList(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(unquote(x)); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		out = "set"
	}
	return out
}
