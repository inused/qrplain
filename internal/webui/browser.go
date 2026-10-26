package webui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

// openBrowser 通过系统 URL 启动器打开地址，超时或启动失败时返回错误。
func openBrowser(parent context.Context, address string) error {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.CommandContext(ctx, "rundll32.exe", "url.dll,FileProtocolHandler", address)
	case "darwin":
		command = exec.CommandContext(ctx, "open", address)
	case "linux":
		command = exec.CommandContext(ctx, "xdg-open", address)
	default:
		return fmt.Errorf("当前系统没有预设浏览器启动方式")
	}
	return command.Run()
}
