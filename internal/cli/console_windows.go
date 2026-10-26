package cli

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/windows"
)

// consoleAttribute 指向 Win32 SetConsoleTextAttribute，用于设置旧控制台颜色。
var consoleAttribute = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetConsoleTextAttribute")

// setConsoleAttribute 设置控制台颜色，失败时返回 Win32 错误。
func setConsoleAttribute(handle windows.Handle, attribute uint16) error {
	result, _, err := consoleAttribute.Call(uintptr(handle), uintptr(attribute))
	if result == 0 {
		return fmt.Errorf("设置控制台颜色失败：%w", err)
	}
	return nil
}

// writeConsole 优先开启 ANSI；旧控制台的 full 模式使用 Win32 颜色 API，退出时恢复原属性。
func writeConsole(file *os.File, modules [][]bool, mode string) (err error) {
	handle := windows.Handle(file.Fd())
	var original uint32
	if err = windows.GetConsoleMode(handle, &original); err != nil {
		return fmt.Errorf("读取控制台模式失败，请保存 PNG：%w", err)
	}
	if windows.SetConsoleMode(handle, original|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil {
		defer func() {
			if restoreErr := windows.SetConsoleMode(handle, original); err == nil {
				err = restoreErr
			}
		}()
		_, err = io.WriteString(file, renderTerminal(modules, mode))
		return err
	}
	if mode != "full" {
		return fmt.Errorf("当前 Windows 控制台不支持 ANSI 半块模式，请使用 --terminal full 或 --out 保存 PNG")
	}
	var info windows.ConsoleScreenBufferInfo
	if err = windows.GetConsoleScreenBufferInfo(handle, &info); err != nil {
		return err
	}
	defer func() {
		if restoreErr := setConsoleAttribute(handle, info.Attributes); err == nil {
			err = restoreErr
		}
	}()
	for _, row := range modules {
		for _, black := range row {
			// Win32 背景颜色位 0x10/0x20/0x40 与高亮位 0x80 合成白底；零值为黑底。
			attribute := uint16(0x00f0)
			if black {
				attribute = 0
			}
			if err = setConsoleAttribute(handle, attribute); err != nil {
				return err
			}
			if _, err = io.WriteString(file, "  "); err != nil {
				return err
			}
		}
		if err = setConsoleAttribute(handle, info.Attributes); err != nil {
			return err
		}
		if _, err = io.WriteString(file, "\n"); err != nil {
			return err
		}
	}
	return nil
}
