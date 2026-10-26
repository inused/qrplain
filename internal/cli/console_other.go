//go:build !windows

package cli

import (
	"io"
	"os"
)

// writeConsole 向 Unix 终端写出 ANSI 二维码。
func writeConsole(file *os.File, modules [][]bool, mode string) error {
	_, err := io.WriteString(file, renderTerminal(modules, mode))
	return err
}
