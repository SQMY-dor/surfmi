# Contributing / 贡献指南

感谢参与！Thanks for helping.

- **Issues**: use the bug report / feature request templates. For conversion bugs, attach a *redacted* profile snippet (remove servers, passwords, UUIDs and subscription tokens).
- **Pull requests**: keep them focused; one topic per PR. Describe what you changed and how you tested it.
- **Go core** (`core/`): run `gofmt`, `go vet -tags with_gvisor ./...` and `go test -tags with_gvisor ./...`. Converter changes need a test case in `core/convert/convert_test.go` and, where useful, a sample in `core/convert/testdata/`. `TestMihomoAcceptsOutput` must keep passing — it feeds every converted sample through mihomo's own parser.
- **Android app** (`app/`): Kotlin official style, Compose + miuix components. Every user-visible string goes into both `values/strings.xml` and `values-zh-rCN/strings.xml`.
- **Clean-room rule**: do **not** submit code copied from, or derived by decompiling, Surge or any other proprietary client. Compatibility work must be based on public documentation only. Code adapted from GPL-compatible projects must keep its attribution.
- By contributing you agree that your contribution is licensed under GPL-3.0-or-later.
