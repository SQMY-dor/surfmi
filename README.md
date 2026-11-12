# Surfmi

> 基于 [mihomo](https://github.com/MetaCubeX/mihomo) 内核、使用 [miuix](https://github.com/compose-miuix-ui/miuix) 风格界面的开源 Android 代理客户端，可直接导入 **Surge 配置**（以及 Clash/mihomo、sing-box、节点链接订阅）。

[English](#english) · [许可证](#许可证--license)

> **免责声明**：Surfmi 是一个独立的开源项目，**与 Surge Networks Inc. 没有任何关联**，也未获得其认可或授权。"Surge" 是其各自所有者的商标。本项目不包含、不复制、也未反编译任何 Surge 代码，仅依据公开文档（[manual.nssurge.com](https://manual.nssurge.com)）实现对 Surge 配置**文件格式**的兼容转换。

---

## 功能

- **mihomo 内核**（v1.19.34），通过 gomobile 编译为 Android 库，Android `VpnService` 创建 TUN 并把 fd 交给 mihomo（gVisor / System / Mixed 协议栈可选）
- 内核自身的连接全部 `protect()`，不会回环进隧道
- **分应用代理**：全部应用 / 仅代理所选应用 / 所选应用绕过
- IPv6 开关、绕过局域网（Android 13+）、自定义 DNS、延迟测试地址
- 前台通知实时显示上下行速度，**快捷设置磁贴**一键开关
- 界面：首页（连接开关、当前配置、规则/全局/直连、流量）、策略组（select / url-test 等，显示延迟、可切换节点）、配置（URL / 文件 / 粘贴导入、更新、编辑源文件、查看生成结果、设为当前）、活动（请求列表 + 日志）、设置（分应用、DNS、主题、语言）
- miuix（HyperOS 风格）界面，主题跟随系统，可选浅色/深色/Monet 动态取色；简体中文 + English
- **内置订阅转换（无需外部 subconverter 服务器）**：
  - Surge `.conf`（含 `#!MANAGED-CONFIG`）与 Surge 节点列表（policy-path 内容）
  - Clash / mihomo YAML（有规则时原样透传）
  - Base64 或明文节点链接：`ss://`、`vmess://`、`vless://`（含 Reality / gRPC / WS / HTTPUpgrade / XHTTP）、`trojan://`、`hysteria2://`/`hy2://`（含端口跳跃）、`hysteria://`、`tuic://`、`anytls://`、`socks5://`、`http(s)://`；`ssr://` 会跳过并提示
  - sing-box JSON 的 outbounds / endpoints（selector、urltest 转为策略组）
  - 源文件没有规则时自动套用默认模板：局域网与中国大陆直连，其余走代理
- 读取 `Subscription-Userinfo` 显示已用流量和到期时间；支持 `profile-title`、`profile-update-interval` 响应头
- 订阅按间隔自动更新（WorkManager，每小时检查一次）
- 支持 `surfmi://install-config?url=…`、`clash://install-config?url=…` 一键导入

## Surge 配置兼容性

| Surge 部分 | 状态 | 说明 |
|---|---|---|
| `[General]` | 部分 | `ipv6`、`loglevel`、`dns-server`、`encrypted-dns-server`、`always-real-ip`、`proxy-test-url`、`allow-wifi-access` 会被转换；`skip-proxy`、`tun-excluded-routes` 等交给 Android VPN 设置处理 |
| `[Proxy]` | ✅ | `ss`（含 obfs、shadow-tls）、`vmess`（WS/TLS）、`trojan`、`snell`（v1–v6）、`http`/`https`、`socks5`/`socks5-tls`、`tuic`/`tuic-v5`、`hysteria2`（含 salamander、端口跳跃）、`wireguard`（`[WireGuard xxx]` 段）、`anytls`、`ssh`（密码）、`direct`/`reject`；通用参数 `udp-relay`、`tfo`、`skip-cert-verify`、`sni`、`underlying-proxy`→`dialer-proxy`、`ip-version`、`server-cert-fingerprint-sha256` |
| `[Proxy Group]` | ✅ | `select`、`url-test`、`fallback`、`load-balance`（`persistent` → 一致性哈希）、`subnet`（降级为手动选择，default 排第一）、`smart`（降级为 url-test）；`policy-path`（下载并内联，失败时改为运行时 proxy-provider）、`include-other-group`、`include-all-proxies`、`policy-regex-filter`、`url`/`interval`/`tolerance`/`timeout`/`hidden` |
| `[Rule]` | ✅ | `DOMAIN`、`DOMAIN-SUFFIX`、`DOMAIN-KEYWORD`、`DOMAIN-WILDCARD`、`IP-CIDR(6)`、`GEOIP`、`IP-ASN`、`SRC-IP`、`DEST-PORT`/`DST-PORT`/`SRC-PORT`/`IN-PORT`（含 `>=` 等表达式）、`PROTOCOL`（TCP/UDP/QUIC）、`PROCESS-NAME`（在 Android 上匹配**包名**）、`AND`/`OR`/`NOT`、`RULE-SET`（远程列表 → classical text rule-provider；`LAN` 展开为私有网段）、`DOMAIN-SET`（→ domain text rule-provider）、`FINAL` → `MATCH` |
| `[Host]` | ✅ | 静态映射 → `hosts`；`server:x.x.x.x` → `nameserver-policy` |
| `[URL Rewrite]` `[Header Rewrite]` `[Body Rewrite]` `[Map Local]` `[MITM]` `[Script]` `[Panel]` | ❌ 不支持 | 转换时给出提示，见下文 |
| `USER-AGENT`、`URL-REGEX`、`SUBNET`、`RULE-SET,SYSTEM` 等规则 | ❌ 跳过 | 转换时给出提示 |

### 为什么 Android 上不支持 MITM / 脚本 / 重写？

1. **MITM（HTTPS 解密）**：需要客户端信任自签 CA。Android 7 起，应用默认**不信任用户安装的 CA 证书**，除非 root 后把证书装入系统分区，绝大多数应用的 HTTPS 无法被解密；在普通设备上实现它既不可靠，也会带来很大的安全风险。
2. **URL/Header/Body 重写与 Map Local**：都依赖对 HTTP(S) 明文的拦截，也就依赖 MITM。
3. **脚本**：Surge 脚本运行在其私有的 JavaScript 运行时和 API（`$httpClient`、`$persistentStore` 等）中，mihomo 没有对应实现；重新实现这一整套非公开语义超出本项目范围。
4. **USER-AGENT / URL-REGEX 规则**：同样需要读取 HTTP 内容。

这些段落会被安全忽略，并在“配置 → 编辑 → 信息”中列出转换提示。

### 其它差异

- `DOMAIN-SET` 中的 `.example.com` 在 Surge 中同时匹配根域，mihomo 的 domain 列表中仅匹配子域（mihomo 需写成 `+.example.com`）。远程列表在运行时由 mihomo 下载，转换器不会改写其内容。
- `policy-path` 指向的 Surge 格式节点列表会在导入/更新时下载并内联；如果下载失败，会退回为 mihomo `proxy-provider`，此时远程内容必须是 Clash YAML 或节点链接。
- 默认模板中的 `GEOSITE`/`GEOIP` 规则首次连接时由 mihomo 自动下载数据库。

## 构建

需要：JDK 17、Go 1.26+（CI 使用 1.27）、Android SDK（compileSdk 37、build-tools 36）、Android NDK（CI 使用 r28c `28.2.13676358`）。

```bash
# 1. 编译内核为 app/libs/libcore.aar（gomobile bind，arm64 / armv7 / x86_64）
export ANDROID_HOME=~/Android/Sdk
export ANDROID_NDK_HOME=$ANDROID_HOME/ndk/28.2.13676358
./scripts/build-core.sh

# 2. 编译 APK
./gradlew :app:assembleDebug        # app/build/outputs/apk/debug/
./gradlew :app:assembleRelease      # 无签名配置时使用 debug key 签名

# 内核与转换器单元测试
cd core && go test -tags with_gvisor ./...

# 桌面端转换工具（与 App 使用同一转换器）
cd core && go run ./cmd/surfmi-convert -fetch my-surge.conf > config.yaml
```

### CI 与签名

`.github/workflows/build.yml` 会：运行 Go 测试 → 用 gomobile 编译内核 → 运行 Android 单元测试 → 构建 arm64-v8a / armeabi-v7a / x86_64 / universal 的 debug 与 release APK 并上传为 Artifact；推送 `v*` 标签时自动创建 GitHub Release。

可选的正式签名（Repository secrets）：`SIGNING_KEYSTORE_BASE64`（`base64 -w0 release.jks`）、`SIGNING_STORE_PASSWORD`、`SIGNING_KEY_ALIAS`、`SIGNING_KEY_PASSWORD`。未配置时 release 包使用 debug key 签名。

### 重命名

应用名称、包名与版本号只在 `gradle.properties` 中定义（`surfmi.appName`、`surfmi.applicationId`、`surfmi.versionName`）。

## 项目结构

```
core/                Go 模块
  convert/           订阅/配置转换器（Surge、Clash、URI、sing-box → mihomo YAML）+ 单元测试与样例
  libcore/           gomobile 桥接：start/stop/reload、流量、策略组、延迟测试、连接、日志、转换
  cmd/surfmi-convert 桌面端转换命令行工具
app/                 Android 应用（Kotlin + Jetpack Compose + miuix）
scripts/build-core.sh
.github/             CI 与 Issue 模板
```

## 许可证 / License

GPL-3.0-or-later，见 [LICENSE](LICENSE)。本项目链接了 GPL-3.0 许可的 mihomo。

致谢：[mihomo](https://github.com/MetaCubeX/mihomo)（GPL-3.0）· [miuix](https://github.com/compose-miuix-ui/miuix)（Apache-2.0）· [ClashMetaForAndroid](https://github.com/MetaCubeX/ClashMetaForAndroid)（GPL-3.0，作为 mihomo Android 集成方式的参考；未复制其代码）。

---

## English

**Surfmi** is an open-source Android proxy client powered by the **mihomo** core with a **miuix** (HyperOS-style) Compose UI. Its main feature is importing **Surge profiles** directly, alongside Clash/mihomo YAML, sing-box JSON and share-link subscriptions — all converted on-device, no external subconverter server.

> **Disclaimer:** Surfmi is an independent project and is **not affiliated with, endorsed by, or sponsored by Surge Networks Inc.** "Surge" is a trademark of its respective owner. No Surge code is used, copied or decompiled; only the publicly documented profile *format* ([manual.nssurge.com](https://manual.nssurge.com)) is implemented for compatibility.

### Features

- mihomo v1.19.34 built with gomobile; Android `VpnService` owns the TUN and hands the fd to mihomo (gVisor / system / mixed stack). All core sockets are `protect()`ed.
- Per-app proxy (all / allow-list / bypass-list), IPv6 toggle, LAN bypass (Android 13+), custom DNS.
- Foreground notification with live speed, Quick Settings tile.
- Screens: Home, Policy groups (latency + selection), Profiles (URL / file / paste, update, edit, set active), Activity (requests + logs), Settings. Theme follows the system (light/dark/Monet), languages: English + 简体中文.
- Built-in conversion: Surge `.conf` and policy lists, Clash/mihomo YAML (pass-through), base64/plain share links (ss, vmess, vless incl. Reality, trojan, hysteria2 with port hopping, hysteria, tuic, anytls, socks5, http; ssr skipped with a warning), sing-box outbounds/endpoints. A default rule template (LAN & mainland China direct, rest proxied) is applied when the source has no rules.
- `Subscription-Userinfo` usage/expiry, `profile-title`, `profile-update-interval`, periodic auto-update.

### Surge compatibility

Supported: `[Proxy]` (ss/obfs/shadow-tls, vmess, trojan, snell v1–v6, http(s), socks5(-tls), tuic(-v5), hysteria2, wireguard, anytls, ssh, direct/reject), `[Proxy Group]` (select, url-test, fallback, load-balance, subnet→select, smart→url-test, policy-path, include-other-group, include-all-proxies, policy-regex-filter), `[Rule]` (DOMAIN*, IP-CIDR(6), GEOIP, IP-ASN, SRC-IP, ports incl. `>=` expressions, PROTOCOL, PROCESS-NAME → Android package name, AND/OR/NOT, RULE-SET → classical text provider, DOMAIN-SET → domain text provider, FINAL → MATCH), `[Host]`, most DNS-related `[General]` keys.

**Not supported on Android:** `[MITM]`, `[Script]`, `[URL Rewrite]`, `[Header Rewrite]`, `[Body Rewrite]`, `[Map Local]`, `[Panel]`, and `USER-AGENT` / `URL-REGEX` rules. HTTPS interception needs a CA that apps trust, but since Android 7 apps ignore user-installed CAs unless the device is rooted; rewrites and Map Local depend on that interception; Surge scripts rely on Surge's proprietary JavaScript runtime and APIs which mihomo does not have. These sections are skipped and listed as conversion notes.

### Build

```bash
export ANDROID_HOME=~/Android/Sdk ANDROID_NDK_HOME=$ANDROID_HOME/ndk/28.2.13676358
./scripts/build-core.sh            # -> app/libs/libcore.aar
./gradlew :app:assembleDebug :app:assembleRelease
(cd core && go test -tags with_gvisor ./...)
```

Requirements: JDK 17, Go ≥ 1.26, Android SDK (compileSdk 37, build-tools 36), NDK. CI builds split + universal APKs (debug and release), uploads them as artifacts and publishes a GitHub Release for `v*` tags. Optional release signing via the `SIGNING_KEYSTORE_BASE64`, `SIGNING_STORE_PASSWORD`, `SIGNING_KEY_ALIAS`, `SIGNING_KEY_PASSWORD` secrets; otherwise the debug key is used. Rename the app in `gradle.properties`.

### License

GPL-3.0-or-later. Credits: mihomo (GPL-3.0), miuix (Apache-2.0), ClashMetaForAndroid (GPL-3.0, consulted as a reference for mihomo's Android hooks; no code copied).
