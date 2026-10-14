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
	"strconv"
	"fmt"
	"sort"
	"strings"

	"github.com/lxn/walk"
)

// hop 保存一跳的聚合数据（对应 Python 版 hops[cur] 字典）。
type hop struct {
	ips []string
	rtt []float64
	n   int
}

// render 渲染实时表格，格式与 tcptrace_gui.py 的 render() 一致。
func render(hops map[int]*hop, stop int) string {
	lines := []string{fmt.Sprintf("%3s  %-18s%9s  Recv", "Hop", "IP", "Avg(ms)")}
	keys := make([]int, 0, len(hops))
	for k := range hops {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, k := range keys {
		if stop > 0 && k > stop {
			break
		}
		h := hops[k]
		if len(h.rtt) > 0 {
			ips := strings.Join(dedup(h.ips), ",")
			avg := 0.0
			for _, v := range h.rtt {
				avg += v
			}
			avg /= float64(len(h.rtt))
			lines = append(lines, fmt.Sprintf("%3d  %-18s%9.2f  %d/%d", k, ips, avg, len(h.rtt), h.n))
		} else {
			lines = append(lines, fmt.Sprintf("%3d  %-18s%9s  0/%d", k, "*", "*", h.n))
		}
	}
	return strings.Join(lines, "\n")
}

func dedup(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// buildConclusion 生成两行结论：判决行（通/半通/不通）+ 诊断行（路径摘要）。
// 路径细节只作为诊断信息出现在第二行。
func buildConclusion(target string, port int, res *traceResult, portRes string) (string, walk.Color) {
	answered := []int{}
	for k, h := range res.hops {
		if len(h.rtt) > 0 && (res.stop == 0 || k <= res.stop) {
			answered = append(answered, k)
		}
	}
	sort.Ints(answered)

	lastIP := "-"
	lastHop := "-"
	if len(answered) > 0 {
		li := answered[len(answered)-1]
		lastHop = strconv.Itoa(li)
		lastIP = strings.Join(dedup(res.hops[li].ips), ",")
	}

	// 诊断行：路径状态摘要
	var diag string
	if res.stop > 0 {
		diag = fmt.Sprintf("路径已到达目标（%d 跳应答）", len(answered))
		if portRes == "TIMEOUT" {
			diag += "，但端口被目标侧过滤"
		}
	} else {
		diag = fmt.Sprintf("路径未到达目标，最后应答第 %s 跳 (%s)", lastHop, lastIP)
		if portRes == "TIMEOUT" {
			diag += "，故障点在其后"
		}
	}

	// 判决行：按端口测试结果
	var msg string
	var color walk.Color
	switch portRes {
	case "OPEN":
		msg, color = fmt.Sprintf("通：%s:%d 端口开放", target, port), walk.RGB(0x08, 0x7f, 0x23)
	case "CLOSED":
		msg, color = fmt.Sprintf("半通：主机可达，端口 %d 未开放（RST 拒绝）", port), walk.RGB(0xb3, 0x6b, 0x00)
	case "TIMEOUT":
		msg, color = fmt.Sprintf("不通：端口 %d 无应答", port), walk.RGB(0xc6, 0x28, 0x28)
	case "UNREACH":
		msg, color = "不通：收到不可达通告", walk.RGB(0xc6, 0x28, 0x28)
	default:
		msg, color = fmt.Sprintf("错误：%s", portRes), walk.RGB(0xc6, 0x28, 0x28)
	}
	return msg + "\n" + diag, color
}
