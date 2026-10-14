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
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

var (
	colBlack  = walk.RGB(0, 0, 0)
	colInput  = walk.RGB(0xff, 0x00, 0x00)
	colRed    = walk.RGB(0xc6, 0x28, 0x28)
	colOrange = walk.RGB(0xb3, 0x6b, 0x00)
	colGreen  = walk.RGB(0x08, 0x7f, 0x23)
)

// app 持有全部 UI 控件与运行状态。
type app struct {
	exePath string

	mw       *walk.MainWindow
	target   *walk.LineEdit
	port     *walk.LineEdit
	btn      *walk.PushButton
	btnCopy  *walk.PushButton
	tv       *walk.TableView
	concl    *walk.CustomWidget
	hopModel *hopTableModel

	// 报告状态（仅 UI 线程读写）
	curTarget  string
	curPort    string
	traceStart time.Time
	reportText string

	// 结论区内容与颜色（PaintBox 自绘使用）
	conclText  string
	conclColor walk.Color

	running bool
	closed  atomic.Bool
	cancel  atomic.Bool
	proc    atomic.Pointer[os.Process]
}

// runUI 创建窗口并进入消息循环。
func runUI(exePath string) error {
	a := &app{exePath: exePath, hopModel: &hopTableModel{}}

	err := MainWindow{
		AssignTo: &a.mw,
		Title:    "TCP Traceroute",
		Font:     Font{Family: "Microsoft YaHei UI", PointSize: 11},
		MinSize:  Size{Width: 660, Height: 640},
		Size:     Size{Width: 660, Height: 640},
		Layout:   VBox{Margins: Margins{Left: 10, Top: 10, Right: 10, Bottom: 8}, Spacing: 6},
		Children: []Widget{
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 6},
				Children: []Widget{
					Label{Text: "目标 IP/域名"},
					LineEdit{AssignTo: &a.target, MaxLength: 253,
						MinSize: Size{Width: 200}, MaxSize: Size{Width: 220}},
					Label{Text: "端口"},
					LineEdit{AssignTo: &a.port, MaxLength: 5,
						MinSize: Size{Width: 70}, MaxSize: Size{Width: 90}},
					PushButton{AssignTo: &a.btn, Text: "开始", OnClicked: a.toggle},
					HSpacer{},
				},
			},
			TableView{
				AssignTo:            &a.tv,
				StretchFactor:       1,
				Columns:             []TableViewColumn{{Title: "Hop", Width: 60}, {Title: "IP", Width: 200}, {Title: "Avg(ms)", Width: 100}, {Title: "Recv"}},
				Model:               a.hopModel,
				CellStyler:          a.hopModel,
				LastColumnStretched: true,
			},
			Composite{
				Layout:  HBox{MarginsZero: true, Spacing: 6},
				MinSize: Size{Height: 52},
				MaxSize: Size{Height: 52},
				Children: []Widget{
					CustomWidget{
						AssignTo:      &a.concl,
						PaintMode:     PaintNormal,
						Paint:         a.paintConcl,
						StretchFactor: 1,
						Font:          Font{Family: "Microsoft YaHei UI", PointSize: 11, Bold: true},
					},
					PushButton{
						AssignTo:  &a.btnCopy,
						Text:      "复制结果",
						Visible:   false,
						OnClicked: a.copyResult,
					},
				},
			},
		},
	}.Create()
	if err != nil {
		return err
	}

	// 标题栏/任务栏图标：从 exe 自身资源加载（go-winres 以 "APP" 嵌入）
	if icon, err := walk.NewIconFromResource("APP"); err == nil {
		_ = a.mw.SetIcon(icon)
	}

	// 回车触发开始/停止；关窗时终止子进程
	onEnter := func(key walk.Key) {
		if key == walk.KeyReturn {
			a.toggle()
		}
	}
	a.target.KeyDown().Attach(onEnter)
	a.port.KeyDown().Attach(onEnter)
	a.mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		a.closed.Store(true)
		a.kill()
	})
	a.target.SetFocus()
	a.mw.Run()
	return nil
}

// ---------- UI 线程方法 ----------

// setConcl 更新结论区内容与颜色（自绘，仅限 UI 线程调用）。
func (a *app) setConcl(text string, color walk.Color) {
	a.conclColor = color
	a.conclText = text
	a.concl.Invalidate()
}

// paintConcl 自绘结论文本：DrawText 自带颜色与自动换行（DT_WORDBREAK）。
// 顶部内缩 4px，避免粗体字上缘被裁切。
func (a *app) paintConcl(canvas *walk.Canvas, damaged walk.Rectangle) error {
	b := a.concl.ClientBounds()
	b.Y += 4
	b.Height -= 4
	return canvas.DrawText(strings.ReplaceAll(a.conclText, "\n", "\r\n"),
		a.concl.Font(), a.conclColor, b,
		walk.TextLeft|walk.TextTop|walk.TextWordbreak|walk.TextNoPrefix)
}

// updateReport 重建可复制的报告文本并联动按钮可见性（仅限 UI 线程调用）。
func (a *app) updateReport(hops map[int]*hop, stop int) {
	if hops == nil {
		a.reportText = ""
		a.btnCopy.SetVisible(false)
		return
	}
	diagIndent := strings.ReplaceAll(a.conclText, "\n", "\r\n     ")
	a.reportText = strings.Join([]string{
		"TCP 路径追踪报告",
		fmt.Sprintf("目标：%s:%s", a.curTarget, a.curPort),
		a.traceStart.Format("时间：2006-01-02 15:04:05"),
		"结果：" + diagIndent,
		"",
		render(hops, stop),
	}, "\r\n")
	a.btnCopy.SetVisible(true)
}

// copyResult 把报告复制到剪贴板，按钮文字短暂反馈结果（仅限 UI 线程调用）。
func (a *app) copyResult() {
	if a.reportText == "" {
		return
	}
	if err := walk.Clipboard().SetText(a.reportText); err != nil {
		a.btnCopy.SetText("复制失败")
	} else {
		a.btnCopy.SetText("已复制 ✓")
	}
	time.AfterFunc(1200*time.Millisecond, func() {
		a.post(func() { a.btnCopy.SetText("复制结果") })
	})
}

func (a *app) done() {
	a.running = false
	a.btn.SetText("开始")
}

// kill 终止子进程并置位取消标记。
func (a *app) kill() {
	a.cancel.Store(true)
	if p := a.proc.Load(); p != nil {
		_ = p.Kill()
	}
}

// toggle 开始/停止（对应 Python toggle）。
func (a *app) toggle() {
	if a.running {
		a.kill()
		return
	}
	target := strings.TrimSpace(a.target.Text())
	if target == "" || strings.HasPrefix(target, "-") || !reHost.MatchString(target) {
		a.setConcl("请输入有效的 IP 或域名", colInput)
		return
	}
	// 端口留空视为 443
	portStr := strings.TrimSpace(a.port.Text())
	if portStr == "" {
		portStr = "443"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		a.setConcl("端口必须是 1-65535 的数字", colInput)
		return
	}
	a.cancel.Store(false)
	a.running = true
	a.btn.SetText("停止")
	a.curTarget = target
	a.curPort = portStr
	a.traceStart = time.Now()
	a.updateReport(nil, 0)
	a.hopModel.update(nil, 0)
	a.setConcl("追踪中...", colBlack)
	go a.worker(target, portStr)
}

// ---------- 后台 goroutine ----------

// worker 对应 tcptrace_gui.py 的 worker：--init → 追踪解析 → 端口测试 → 结论。
func (a *app) worker(target, port string) {
	runInit(a.exePath)

	res := runTrace(a.exePath, target, port, &a.cancel, traceHooks{
		onProcess: func(p *os.Process) { a.proc.Store(p) },
		onTable: func(hops map[int]*hop, stop int) {
			a.post(func() {
				a.hopModel.update(hops, stop)
				a.updateReport(hops, stop)
			})
		},
	})

	// setUI 把更新投递到 UI 线程；窗口已关闭则静默丢弃。
	setUI := func(f func()) {
		if !a.closed.Load() {
			a.mw.Synchronize(f)
		}
	}

	switch {
	case res.err != nil:
		msg := fmt.Sprintf("运行出错：%v", res.err)
		setUI(func() { a.setConcl(msg, colRed); a.done(); a.updateReport(nil, 0) })
	case res.canceled:
		setUI(func() {
			a.setConcl("已停止", colBlack)
			a.done()
			if len(res.hops) > 0 {
				a.updateReport(res.hops, res.stop)
			} else {
				a.updateReport(nil, 0)
			}
		})
	case len(res.hops) == 0:
		tail := res.raw
		if len(tail) > 3 {
			tail = tail[len(tail)-3:]
		}
		tailStr := strings.Join(tail, " | ")
		if tailStr == "" {
			tailStr = "无输出"
		}
		msg := fmt.Sprintf("没有得到追踪结果（请确认以管理员运行、WinDivert 已就绪）：%s", tailStr)
		setUI(func() { a.setConcl(msg, colRed); a.done(); a.updateReport(nil, 0) })
	default:
		setUI(func() {
			a.hopModel.update(res.hops, res.stop)
			a.setConcl("追踪完成，正在测试端口连接...", colBlack)
		})
		portNum := atoi(port)
		portRes := portTest(target, portNum)
		text, color := buildConclusion(target, portNum, res, portRes)
		setUI(func() { a.setConcl(text, color); a.done(); a.updateReport(res.hops, res.stop) })
	}
}

// post 与 setUI 等价，供 onTable 回调使用。
func (a *app) post(f func()) {
	if !a.closed.Load() {
		a.mw.Synchronize(f)
	}
}
