// QRPlain 提供本地离线二维码生成与解析，支持浏览器界面和 CLI。
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"qrplain/internal/cli"
	"qrplain/internal/webui"
)

// version 可通过构建参数 -ldflags "-X main.version=版本号" 注入；开发构建使用 dev。
var version = "dev"

// main 分派命令，并在浏览器模式中处理退出信号。
func main() {
	args := os.Args[1:]
	if len(args) == 1 {
		switch args[0] {
		case "--help", "-h", "help":
			if _, err := fmt.Fprint(os.Stdout, cli.Help); err != nil {
				os.Exit(1)
			}
			return
		case "--version":
			if _, err := fmt.Fprintln(os.Stdout, "qrplain", version); err != nil {
				os.Exit(1)
			}
			return
		}
	}
	if len(args) == 0 || (len(args) == 1 && args[0] == "ui") {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		if err := webui.Run(ctx, os.Stderr); err != nil {
			fmt.Fprintln(os.Stderr, "错误：", err)
			os.Exit(1)
		}
		return
	}
	os.Exit(cli.Run(args, os.Stdout, os.Stderr))
}
