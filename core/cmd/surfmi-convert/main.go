// SPDX-License-Identifier: GPL-3.0-or-later

// Command surfmi-convert converts a subscription/profile into mihomo YAML on
// the desktop, using the same converter as the Android app.
//
//	surfmi-convert [-fetch] [-template] [-test-url URL] <file|url|->
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/SQMY-dor/surfmi/core/convert"
)

func main() {
	fetch := flag.Bool("fetch", false, "download Surge policy-path lists and inline them")
	template := flag.Bool("template", false, "replace source rules with the default template")
	testURL := flag.String("test-url", "", "latency test URL for generated groups")
	ua := flag.String("ua", "Surfmi/0.1", "User-Agent for remote downloads")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: surfmi-convert [flags] <file|url|->")
		os.Exit(2)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	get := func(u string) ([]byte, error) {
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", *ua)
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if h := resp.Header.Get("Subscription-Userinfo"); h != "" {
			fmt.Fprintf(os.Stderr, "userinfo: %s\n", convert.ParseUserinfo(h).JSON())
		}
		if resp.StatusCode/100 != 2 {
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return io.ReadAll(resp.Body)
	}

	src := flag.Arg(0)
	var (
		data []byte
		err  error
	)
	switch {
	case src == "-":
		data, err = io.ReadAll(os.Stdin)
	case strings.HasPrefix(src, "http://"), strings.HasPrefix(src, "https://"):
		data, err = get(src)
	default:
		data, err = os.ReadFile(src)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}
	opt := convert.Options{TestURL: *testURL, ForceTemplate: *template}
	if *fetch {
		opt.Fetcher = convert.FetcherFunc(get)
	}
	res, err := convert.Convert(data, opt)
	if err != nil {
		fmt.Fprintln(os.Stderr, "convert:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "format=%s proxies=%d groups=%d rules=%d\n", res.Format, res.Proxies, res.Groups, res.Rules)
	for _, w := range res.Warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	fmt.Print(res.YAML)
}
