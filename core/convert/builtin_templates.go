// SPDX-License-Identifier: GPL-3.0-or-later

package convert

import (
	"embed"
	"encoding/json"
)

//go:embed templates/simple.yaml templates/split.yaml templates/surge_example.conf
var templateFS embed.FS

// BuiltinTemplate is a rule template shipped with the app.
type BuiltinTemplate struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	NameEN        string `json:"nameEn"`
	Description   string `json:"description"`
	DescriptionEN string `json:"descriptionEn"`
	Content       string `json:"content"`
}

var builtinTemplateMeta = []BuiltinTemplate{
	{ID: "builtin:simple", Name: "简洁", NameEN: "Simple",
		Description:   "节点选择 / 自动选择 / 漏网之鱼，国内直连，其余代理",
		DescriptionEN: "Proxy / Auto / Final; China direct, everything else proxied"},
	{ID: "builtin:split", Name: "分流", NameEN: "Split",
		Description:   "在简洁基础上增加 AI、流媒体、Telegram、谷歌、苹果、微软、广告拦截及香港/台湾/日本/新加坡/美国地区分组",
		DescriptionEN: "Adds AI, streaming, Telegram, Google, Apple, Microsoft, ad-block and HK/TW/JP/SG/US region groups"},
	{ID: "builtin:surge-example", Name: "Surge 语法示例", NameEN: "Surge syntax example",
		Description:   "使用 [Proxy Group] / [Rule] 写法和 policy-regex-filter 的示例",
		DescriptionEN: "Example using [Proxy Group] / [Rule] sections and policy-regex-filter"},
}

var builtinTemplateFiles = map[string]string{
	"builtin:simple":        "templates/simple.yaml",
	"builtin:split":         "templates/split.yaml",
	"builtin:surge-example": "templates/surge_example.conf",
}

// BuiltinTemplates returns the templates shipped with the app.
func BuiltinTemplates() []BuiltinTemplate {
	out := make([]BuiltinTemplate, 0, len(builtinTemplateMeta))
	for _, t := range builtinTemplateMeta {
		b, err := templateFS.ReadFile(builtinTemplateFiles[t.ID])
		if err != nil {
			continue
		}
		t.Content = string(b)
		out = append(out, t)
	}
	return out
}

// BuiltinTemplateContent returns the content of a built-in template by id.
func BuiltinTemplateContent(id string) (string, bool) {
	f, ok := builtinTemplateFiles[id]
	if !ok {
		return "", false
	}
	b, err := templateFS.ReadFile(f)
	if err != nil {
		return "", false
	}
	return string(b), true
}

// BuiltinTemplatesJSON encodes BuiltinTemplates as JSON.
func BuiltinTemplatesJSON() string {
	b, _ := json.Marshal(BuiltinTemplates())
	return string(b)
}
