@echo off
setlocal
cd /d "%~dp0"
set "PATH=D:\Go\bin;%USERPROFILE%\go\bin;%PATH%"

if not exist ..\nexttrace-tiny_windows_amd64.exe (
  echo Put nexttrace-tiny_windows_amd64.exe in the parent folder first.
  pause & exit /b 1
)

rem 1. Copy nexttrace tiny into assets\ (go:embed cannot use ".." paths)
copy /y ..\nexttrace-tiny_windows_amd64.exe assets\nexttrace_windows_amd64.exe >nul

rem 2. Generate resources (requireAdministrator + comctl32 v6 + DPI manifest)
where go-winres >nul 2>nul || go install github.com/tc-hib/go-winres@latest
go-winres make --arch amd64
if errorlevel 1 (echo Resource generate FAILED & pause & exit /b 1)

rem 3. SHA-256 of the embedded file, injected via ldflags for first-run check
set "EMBED_HASH="
for /f "skip=1 delims=" %%h in ('certutil -hashfile assets\nexttrace_windows_amd64.exe SHA256') do (
  if not defined EMBED_HASH set "EMBED_HASH=%%h"
)

rem 4. Build (windowsgui: no console window)
if not exist ..\dist mkdir ..\dist
go build -trimpath -ldflags "-s -w -H windowsgui -X main.embeddedHash=%EMBED_HASH%" -o ..\dist\TCPTrace.exe .
if errorlevel 1 (echo Build FAILED & pause & exit /b 1)

echo Build OK: ..\dist\TCPTrace.exe
pause
