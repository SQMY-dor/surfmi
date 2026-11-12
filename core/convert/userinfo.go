// SPDX-License-Identifier: GPL-3.0-or-later

package convert

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Userinfo is the parsed Subscription-Userinfo response header
// ("upload=1; download=2; total=3; expire=1700000000").
type Userinfo struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
	Total    int64 `json:"total"`
	Expire   int64 `json:"expire"` // unix seconds, 0 = unknown/never
}

// ParseUserinfo parses a Subscription-Userinfo header value. Unknown or
// malformed fields are ignored.
func ParseUserinfo(header string) Userinfo {
	var u Userinfo
	for _, part := range strings.Split(header, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			continue
		}
		n := int64(f)
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "upload":
			u.Upload = n
		case "download":
			u.Download = n
		case "total":
			u.Total = n
		case "expire":
			u.Expire = n
		}
	}
	return u
}

func (u Userinfo) JSON() string {
	b, _ := json.Marshal(u)
	return string(b)
}
