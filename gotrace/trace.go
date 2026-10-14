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
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const createNoWindow = 0x08000000

var (
	reANSI = regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]")
	reIP   = regexp.MustCompile(`\d{1,3}(?:\.\d{1,3}){3}`)
	reHost = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.\-]*[A-Za-z0-9])?$`)
	reHop  = regexp.MustCompile(`^(\d+)\s+(.*)$`)
	reCont = regexp.MustCompile(`^\s+\S`)
	reRTT  = regexp.MustCompile(`^(\d{1,3}(?:\.\d{1,3}){3})\s+([\d.]+)\s*ms`)
	reDest = regexp.MustCompile(`->\s+.*?(\d{1,3}(?:\.\d{1,3}){3})`)
)

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

// traceParser 逐行解析 nexttrace 输出，状态机与 tcptrace_gui.py 的 worker 一致。
type traceParser struct {
	hops map[int]*hop
	raw  []string
	cur  int
	rest string
	dest string
	stop int // 0 = 尚未到达目标
	done bool
}

// feed 处理一行输出；返回 killNow=true 表示已过 stop 跳，应立即终止子进程。
func (p *traceParser) feed(raw string) (updated, killNow bool) {
	if p.done {
		return false, false
	}
	line := reANSI.ReplaceAllString(strings.TrimRight(raw, "\r\n"), "")
	if strings.TrimSpace(line) == "" {
		return false, false
	}
	p.raw = append(p.raw, line)
	if p.dest == "" && strings.Contains(line, "hops max") {
		if m := reDest.FindStringSubmatch(line); m != nil {
			p.dest = m[1]
		}
		return false, false
	}
	if m := reHop.FindStringSubmatch(line); m != nil {
		cur, _ := strconv.Atoi(m[1])
		if p.stop > 0 && cur > p.stop {
			p.done = true
			return false, true
		}
		p.cur = cur
		p.rest = m[2]
		if _, ok := p.hops[cur]; !ok {
			p.hops[cur] = &hop{}
		}
	} else if p.cur > 0 && reCont.MatchString(line) {
		p.rest = strings.TrimSpace(line)
	} else {
		return false, false
	}
	h := p.hops[p.cur]
	h.n++
	if m := reRTT.FindStringSubmatch(p.rest); m != nil {
		rtt, _ := strconv.ParseFloat(m[2], 64)
		h.ips = append(h.ips, m[1])
		h.rtt = append(h.rtt, rtt)
		if m[1] == p.dest && p.stop == 0 {
			p.stop = p.cur
		}
	}
	return true, false
}

// lineWriter 把字节流按行拆开回调（stdout/stderr 各用一个实例、共用同一个
// 回调即完成合并）。Write 本身不加锁：并发安全由回调内部的锁保证。
type lineWriter struct {
	on  func(line string)
	buf []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(string(w.buf[:i]), "\r")
		w.buf = w.buf[i+1:]
		w.on(line)
	}
	return len(p), nil
}

type traceResult struct {
	hops     map[int]*hop
	dest     string
	stop     int
	raw      []string
	canceled bool
	err      error
}

type traceHooks struct {
	onProcess func(*os.Process)
	onTable   func(hops map[int]*hop, stop int)
}

// runTrace 启动 nexttrace 并解析其输出，返回汇总结果。
func runTrace(exePath, target, port string, cancelFlag *atomic.Bool, hooks traceHooks) *traceResult {
	cmd := exec.Command(exePath, "-T", "-p", port, "-d", "disable-geoip",
		"-n", "-c", "-C", "-g", "en", target)
	cmd.Dir = filepath.Dir(exePath)
	hideWindow(cmd)

	p := &traceParser{hops: map[int]*hop{}}
	var mu sync.Mutex
	killOnce := false
	// snapshot 深拷贝当前跳数据：onTable 回调把数据交给 UI 线程渲染，
	// 而 parser 会被 reader goroutine 持续写入，不拷贝会构成数据竞争。
	snapshot := func() map[int]*hop {
		snap := make(map[int]*hop, len(p.hops))
		for k, h := range p.hops {
			cp := &hop{n: h.n}
			cp.ips = append(cp.ips, h.ips...)
			cp.rtt = append(cp.rtt, h.rtt...)
			snap[k] = cp
		}
		return snap
	}
	onLine := func(line string) {
		mu.Lock()
		defer mu.Unlock()
		updated, killNow := p.feed(line)
		if killNow && !killOnce {
			killOnce = true
			_ = cmd.Process.Kill()
		}
		if updated && !killOnce {
			hooks.onTable(snapshot(), p.stop)
		}
	}
	cmd.Stdout = &lineWriter{on: onLine}
	cmd.Stderr = &lineWriter{on: onLine}

	if err := cmd.Start(); err != nil {
		return &traceResult{err: err}
	}
	if hooks.onProcess != nil {
		hooks.onProcess(cmd.Process)
	}

	// 连带释放：挂入 KILL_ON_JOB_CLOSE 的作业对象。封装进程无论正常退出、
	// 崩溃还是被任务管理器强杀，作业句柄随进程消亡自动关闭，内核强制
	// 终止 nexttrace，杜绝孤儿进程。
	if job, jerr := windows.CreateJobObject(nil, nil); jerr == nil {
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		ret, ierr := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
		if ret != 0 && ierr == nil {
			// Go 1.26 移除了 os.Process.Handle()，按 PID 重新打开句柄
			if ph, perr := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
				false, uint32(cmd.Process.Pid)); perr == nil {
				_ = windows.AssignProcessToJobObject(job, ph)
				windows.CloseHandle(ph)
			}
		}
		defer windows.CloseHandle(job)
	}

	// 与 Python 版一致：读到 EOF（进程退出）为止，不设人为超时
	// （真实追踪可持续数十秒，5 秒超时会在追踪中途误杀进程）
	_ = cmd.Wait()

	return &traceResult{
		hops:     p.hops,
		dest:     p.dest,
		stop:     p.stop,
		raw:      p.raw,
		canceled: cancelFlag.Load(),
	}
}

// consoleRun 是 -console 调试模式：跳过 GUI，直接在控制台验证追踪与解析逻辑。
func consoleRun() {
	args := os.Args[2:]
	if len(args) < 2 {
		fmt.Println("用法: TCPTrace.exe -console <target> <port>")
		return
	}
	exePath, err := EnsureNexttrace()
	if err != nil {
		fmt.Println("初始化失败:", err)
		return
	}
	fmt.Println("nexttrace:", exePath)
	runInit(exePath)
	var cancel atomic.Bool
	res := runTrace(exePath, args[0], args[1], &cancel, traceHooks{
		onTable: func(hops map[int]*hop, stop int) {
			fmt.Println(render(hops, stop))
		},
	})
	if res.err != nil {
		fmt.Println("出错:", res.err)
		return
	}
	port := atoi(args[1])
	portRes := portTest(args[0], port)
	text, _ := buildConclusion(args[0], port, res, portRes)
	fmt.Println("\n" + text)
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
