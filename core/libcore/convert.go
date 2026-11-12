// SPDX-License-Identifier: GPL-3.0-or-later

package libcore

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/SQMY-dor/surfmi/core/convert"
)

// UserAgent is sent when the converter fetches policy-path resources.
var UserAgent = "Surfmi/" + WrapperVersion

type httpFetcher struct{ client *http.Client }

func (f httpFetcher) Fetch(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 16<<20))
}

// ConvertProfile converts subscription content (Surge conf, Clash YAML,
// URI list, sing-box JSON) into mihomo YAML. It returns the convert.Result
// as JSON: {format, yaml, warnings[], proxies, groups, rules, managed}.
// When fetchRemote is true, Surge policy-path lists are downloaded and
// inlined.
func ConvertProfile(data []byte, testURL string, fetchRemote bool) (string, error) {
	opt := convert.Options{TestURL: testURL}
	if fetchRemote {
		opt.Fetcher = httpFetcher{client: &http.Client{Timeout: 20 * time.Second}}
	}
	res, err := convert.Convert(data, opt)
	if err != nil {
		return "", err
	}
	return res.JSON(), nil
}

// DetectFormat returns the detected format name of data.
func DetectFormat(data []byte) string { return string(convert.Detect(data)) }

// ParseUserinfo parses a Subscription-Userinfo header into JSON
// {upload, download, total, expire}.
func ParseUserinfo(header string) string { return convert.ParseUserinfo(header).JSON() }
