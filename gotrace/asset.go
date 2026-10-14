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
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed assets/nexttrace_windows_amd64.exe
var nexttraceBin []byte

// embeddedHash 由构建脚本经 -ldflags "-X main.embeddedHash=..." 注入（小写 hex），
// 避免启动时对 33MB 内存数据做哈希。
var embeddedHash = ""

const exeName = "nexttrace_windows_amd64.exe"

func appDir() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "TCPTrace")
}

var hashOnce sync.Once
var wantHashVal string

// wantHash 返回内嵌数据的 SHA-256（小写 hex）；构建时未注入则首调时计算。
func wantHash() string {
	hashOnce.Do(func() {
		if embeddedHash != "" {
			wantHashVal = embeddedHash
			return
		}
		sum := sha256.Sum256(nexttraceBin)
		wantHashVal = hex.EncodeToString(sum[:])
	})
	return wantHashVal
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// EnsureNexttrace 把内嵌的 nexttrace 释放到 %LOCALAPPDATA%\TCPTrace。
// 已存在且大小、SHA-256 均一致时直接复用（二次启动零解压，毫秒级）。
func EnsureNexttrace() (string, error) {
	dir := appDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, exeName)
	if fi, err := os.Stat(dst); err == nil && fi.Size() == int64(len(nexttraceBin)) {
		if sum, err := fileSHA256(dst); err == nil && strings.EqualFold(sum, wantHash()) {
			return dst, nil
		}
	}
	// 不存在、大小不符或哈希不符：写临时文件后替换
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, nexttraceBin, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return "", errors.New(
			"无法写入 "+dst+"（文件可能被其他 TCPTrace 实例占用，请关闭后重试）："+err.Error())
	}
	return dst, nil
}

// runInit 让 nexttrace 释放 WinDivert 运行时到其所在目录。
// 与 Python 版一致：每次追踪前执行、忽略一切错误、30 秒超时。
func runInit(exePath string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exePath, "--init")
	cmd.Dir = filepath.Dir(exePath)
	hideWindow(cmd)
	_ = cmd.Run()
}
