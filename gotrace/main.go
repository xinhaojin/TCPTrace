// Copyright (C) 2026 TCPTrace contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"fmt"
	"os"

	"github.com/lxn/walk"
)

func main() {
	// 调试/验证用控制台模式：TCPTrace.exe -console <target> <port>
	if len(os.Args) > 1 && os.Args[1] == "-console" {
		consoleRun()
		return
	}
	exePath, err := EnsureNexttrace()
	if err != nil {
		showErr(err)
		return
	}
	if err := runUI(exePath); err != nil {
		showErr(err)
	}
}

// showErr 在无控制台的 windowsgui 模式下用 MessageBox 报告致命错误。
func showErr(err error) {
	msg := fmt.Sprintf("TCPTrace 启动失败：\n\n%v", err)
	walk.MsgBox(nil, "TCPTrace", msg, walk.MsgBoxIconError)
}
