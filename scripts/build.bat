@echo off
setlocal
rem 使用当前 Go 工具链构建六种平台产物；失败时返回非零退出码。
rem 按脚本位置定位项目根目录，退出时恢复调用方环境和工作目录。
pushd "%~dp0.."
if errorlevel 1 exit /b 1

if not exist "dist\" mkdir "dist"
if errorlevel 1 goto failed
if not exist "dist\docs\" mkdir "dist\docs"
if errorlevel 1 goto failed
rem 许可文件随可执行文件分发。
copy /y "LICENSE" "dist\LICENSE"
if errorlevel 1 goto failed
copy /y "docs\THIRD_PARTY.md" "dist\docs\THIRD_PARTY.md"
if errorlevel 1 goto failed
set "CGO_ENABLED=0"

for %%T in (windows/amd64 windows/arm64 linux/amd64 linux/arm64 darwin/amd64 darwin/arm64) do (
    for /f "tokens=1,2 delims=/" %%O in ("%%T") do (
        call :build %%O %%P
        if errorlevel 1 goto failed
    )
)
popd
exit /b 0

:build
rem 参数依次为 GOOS、GOARCH；Windows 产物使用 .exe 扩展名。
set "GOOS=%~1"
set "GOARCH=%~2"
set "extension="
if "%GOOS%"=="windows" set "extension=.exe"
go build -trimpath -ldflags "-s -w" -o "dist\qrplain-%GOOS%-%GOARCH%%extension%" .
exit /b %errorlevel%

:failed
rem 构建失败后恢复调用方工作目录。
popd
exit /b 1
