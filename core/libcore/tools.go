// SPDX-License-Identifier: GPL-3.0-or-later

//go:build tools

package libcore

// Keeps golang.org/x/mobile in go.mod; gomobile bind needs its bind package
// resolvable from this module.
import _ "golang.org/x/mobile/bind"
