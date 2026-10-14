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
	"errors"
	"net"
	"strconv"
	"syscall"
	"time"
)

// portTest TCP 连通性测试（对应 tcptrace_gui.py 的 port_test），
// 返回 OPEN / CLOSED / TIMEOUT / UNREACH / ERROR:<msg>。
func portTest(host string, port int) string {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 3*time.Second)
	if err == nil {
		conn.Close()
		return "OPEN"
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case 10061: // WSAECONNREFUSED
			return "CLOSED"
		case 10065, 10051: // WSAEHOSTUNREACH / WSAENETUNREACH
			return "UNREACH"
		case 10060: // WSAETIMEDOUT
			return "TIMEOUT"
		}
	}
	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		return "TIMEOUT"
	}
	return "ERROR:" + err.Error()
}
