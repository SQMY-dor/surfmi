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
	return ConvertProfileWithTemplate(data, testURL, fetchRemote, "", "")
}

// ConvertProfileWithTemplate is ConvertProfile plus a local rule template
// ("规则组替换"). template is the template text (Surge [Proxy Group]/[Rule]
// snippet or Clash YAML with proxy-groups/rules/rule-providers), or a
// built-in id such as "builtin:simple"; empty means no override. mode is
// "replace" (default: keep only the subscription's proxies), "append" or
// "prepend" (keep the subscription's groups/rules and add the template's).
func ConvertProfileWithTemplate(data []byte, testURL string, fetchRemote bool, template, mode string) (string, error) {
	opt := convert.Options{TestURL: testURL}
	if template != "" {
		if c, ok := convert.BuiltinTemplateContent(template); ok {
			template = c
		}
		opt.Override = &convert.Override{Template: template, Mode: convert.ParseOverrideMode(mode)}
	}
	if fetchRemote {
		opt.Fetcher = httpFetcher{client: &http.Client{Timeout: 20 * time.Second}}
	}
	res, err := convert.Convert(data, opt)
	if err != nil {
		return "", err
	}
	return res.JSON(), nil
}

// BuiltinTemplates returns the built-in rule templates as a JSON array of
// {id, name, nameEn, description, descriptionEn, content}.
func BuiltinTemplates() string { return convert.BuiltinTemplatesJSON() }

// ValidateTemplate parses a rule template and returns "" when it is valid,
// otherwise the error message. Warnings are not reported here.
func ValidateTemplate(template string) string {
	if c, ok := convert.BuiltinTemplateContent(template); ok {
		template = c
	}
	if _, err := convert.ParseTemplate(template); err != nil {
		return err.Error()
	}
	return ""
}

// DetectFormat returns the detected format name of data.
func DetectFormat(data []byte) string { return string(convert.Detect(data)) }

// ParseUserinfo parses a Subscription-Userinfo header into JSON
// {upload, download, total, expire}.
func ParseUserinfo(header string) string { return convert.ParseUserinfo(header).JSON() }
