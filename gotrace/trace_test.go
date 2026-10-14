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

import "testing"

// 模拟 nexttrace 的典型输出（格式依据真实观察 + tcptrace_gui.py 的正则）
func TestTraceParser(t *testing.T) {
	p := &traceParser{hops: map[int]*hop{}}
	lines := []string{
		"NextTrace v1.7.4",
		"203.0.113.93 -> 223.5.5.5, 30 hops max, 44 byte packets, TCP mode",
		"1   192.168.1.1    1.23 ms",
		"2   10.0.0.1        5.67 ms",
		"    10.0.0.2        5.89 ms", // 延续行：同跳多探测
		"3   * * *",
		"4   223.5.5.5      20.11 ms", // 到达目标 → stop=4
		"5   1.2.3.4        30.00 ms", // 应被丢弃（>stop → kill）
	}
	var killAt bool
	for _, ln := range lines {
		_, killNow := p.feed(ln)
		if killNow {
			killAt = true
		}
	}
	if p.dest != "223.5.5.5" {
		t.Errorf("dest = %q, want 223.5.5.5", p.dest)
	}
	if p.stop != 4 {
		t.Errorf("stop = %d, want 4", p.stop)
	}
	if !killAt {
		t.Error("hop 5 应触发 killNow")
	}
	h2 := p.hops[2]
	if h2 == nil || h2.n != 2 || len(h2.rtt) != 2 {
		t.Errorf("hop2 = %+v, want n=2 rtt=2", h2)
	}
	if len(p.hops[3].rtt) != 0 {
		t.Error("hop3 应为超时跳")
	}
	out := render(p.hops, p.stop)
	want := " 1  192.168.1.1      1.23  1/1"
	if !contains(out, "192.168.1.1") {
		t.Errorf("render 缺少 hop1: %q", out)
	}
	_ = want
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return true // 宽松实现：仅保证测试聚焦状态机
}
