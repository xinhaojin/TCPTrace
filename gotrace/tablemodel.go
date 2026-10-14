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
	"sort"
	"strconv"
	"strings"

	"github.com/lxn/walk"
)

var (
	colGray     = walk.RGB(0x99, 0x99, 0x99)
	boldFont, _ = walk.NewFont("Microsoft YaHei UI", 11, walk.FontBold)
)

// hopRow 是表格一行的展示数据。
type hopRow struct {
	hop   int
	ip    string
	avg   string
	recv  string
	color walk.Color
	bold  bool
}

// hopTableModel 供 walk TableView 使用：持有行数据并按行着色。
type hopTableModel struct {
	walk.TableModelBase
	rows []hopRow
}

func (m *hopTableModel) RowCount() int { return len(m.rows) }

func (m *hopTableModel) Value(r, c int) interface{} {
	row := m.rows[r]
	switch c {
	case 0:
		return row.hop
	case 1:
		return row.ip
	case 2:
		return row.avg
	default:
		return row.recv
	}
}

// StyleCell 实现 walk.CellStyler：超时行灰、目标行绿加粗、
// 未到达时的最后应答行橙，其余黑。
func (m *hopTableModel) StyleCell(style *walk.CellStyle) {
	row := m.rows[style.Row()]
	style.TextColor = row.color
	if row.bold {
		style.Font = boldFont
	}
}

// update 按当前跳数据重建行并刷新表格（仅限 UI 线程调用）。
func (m *hopTableModel) update(hops map[int]*hop, stop int) {
	m.rows = buildHopRows(hops, stop)
	m.PublishRowsReset()
}

// buildHopRows 把解析结果转换为表格行（着色规则见 StyleCell）。
func buildHopRows(hops map[int]*hop, stop int) []hopRow {
	keys := make([]int, 0, len(hops))
	for k := range hops {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	// 未到达时，最后有应答的跳用橙色提示故障位置
	lastAnswered := 0
	for _, k := range keys {
		if len(hops[k].rtt) > 0 && (stop == 0 || k <= stop) {
			lastAnswered = k
		}
	}

	rows := make([]hopRow, 0, len(keys))
	for _, k := range keys {
		if stop > 0 && k > stop {
			break
		}
		h := hops[k]
		row := hopRow{hop: k, color: colBlack}
		if len(h.rtt) == 0 {
			row.ip, row.avg, row.recv, row.color = "*", "*", "0/"+strconv.Itoa(h.n), colGray
		} else {
			avg := 0.0
			for _, v := range h.rtt {
				avg += v
			}
			avg /= float64(len(h.rtt))
			row.ip = strings.Join(dedup(h.ips), ",")
			row.avg = fmt.Sprintf("%.2f", avg)
			row.recv = fmt.Sprintf("%d/%d", len(h.rtt), h.n)
			if stop > 0 && k == stop {
				row.color, row.bold = colGreen, true
			} else if stop == 0 && k == lastAnswered && k != keys[len(keys)-1] {
				row.color = colOrange
			}
		}
		rows = append(rows, row)
	}
	return rows
}
