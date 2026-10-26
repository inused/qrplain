// Package cli 提供二维码命令行入口、退出码和终端显示。
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"qrplain/internal/qr"

	"golang.org/x/term"
)

// Help 是主入口与子命令共用的中文用法说明。
const Help = `QRPlain · 本地二维码

用法：
  qrplain                         打开本地浏览器界面
  qrplain ui                      打开本地浏览器界面
  qrplain encode --text "正文"     在终端显示二维码
  qrplain encode --file 正文.txt   读取 UTF-8 文件并在终端显示
  qrplain encode --text "正文" --out qr.png --size 512 --level M
  qrplain decode --image qr.png    识别 PNG/JPEG 图片
  qrplain --help                  显示帮助
  qrplain --version               显示版本

生成选项：
  --text string       正文，与 --file 互斥
  --file string       UTF-8 正文文件路径
  --out string        PNG 保存路径；已有文件报错，省略时显示终端二维码
  --size int          PNG 边长，默认 512，最大 4096 像素
  --level string      纠错等级 L/M/Q/H，默认 M
  --terminal string   half（默认半块）或 full（每模块占两列）

退出码：0 成功；1 处理失败；2 参数错误。
`

// Run 执行 encode 或 decode；参数错误返回 2，处理或写入失败返回 1。
//
//	@param args []string 包含子命令及其参数，不含程序名。
//	@param out io.Writer 二维码或识别正文的输出位置。
//	@param errout io.Writer 错误、帮助及终端提示的输出位置。
//	@return int 进程应使用的退出码。
func Run(args []string, out, errout io.Writer) int {
	if len(args) == 0 {
		return fail(errout, 2, fmt.Errorf("缺少子命令"))
	}
	if args[0] != "encode" && args[0] != "decode" {
		return fail(errout, 2, fmt.Errorf("未知子命令 %q，请使用 --help", args[0]))
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(errout)
	fs.Usage = func() { fmt.Fprint(errout, Help) }
	var text, file, output, imagePath, level, mode string
	size := qr.DefaultSize
	if args[0] == "encode" {
		fs.StringVar(&text, "text", "", "正文")
		fs.StringVar(&file, "file", "", "UTF-8 文件")
		fs.StringVar(&output, "out", "", "PNG 输出路径")
		fs.IntVar(&size, "size", qr.DefaultSize, "PNG 边长")
		fs.StringVar(&level, "level", "M", "纠错等级")
		fs.StringVar(&mode, "terminal", "half", "终端呈现方式")
	} else {
		fs.StringVar(&imagePath, "image", "", "PNG/JPEG 路径")
	}
	// flag 会把下一项选项当作字符串值，这里据选项名检查缺值。
	// 正文本身是选项名时使用 --text=--out 的等号形式。
	for i := 1; i+1 < len(args); i++ {
		if (args[i] == "--text" || args[i] == "-text" || args[i] == "--file" || args[i] == "-file" || args[i] == "--out" || args[i] == "-out" || args[i] == "--image" || args[i] == "-image") && knownOption(fs, args[i+1]) {
			return fail(errout, 2, fmt.Errorf("%s 缺少值；正文以选项名开头时请使用等号传值", args[i]))
		}
	}
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		return fail(errout, 2, fmt.Errorf("未识别的参数 %q；请检查选项和值的顺序", fs.Args()))
	}
	if args[0] == "decode" {
		if imagePath == "" {
			return fail(errout, 2, fmt.Errorf("必须使用 --image 指定图片"))
		}
		f, err := os.Open(imagePath)
		if err != nil {
			return fail(errout, 1, fmt.Errorf("打开图片失败：%w", err))
		}
		defer f.Close()
		content, err := qr.Decode(f)
		if err != nil {
			return fail(errout, 1, err)
		}
		if _, err = fmt.Fprintln(out, content); err != nil {
			return fail(errout, 1, fmt.Errorf("输出正文失败：%w", err))
		}
		return 0
	}
	provided := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { provided[f.Name] = true })
	if provided["text"] == provided["file"] {
		return fail(errout, 2, fmt.Errorf("必须且只能指定 --text、--file 其中一种输入"))
	}
	if (provided["text"] && text == "") || (provided["file"] && file == "") {
		return fail(errout, 2, fmt.Errorf("--text 或 --file 的值不能为空"))
	}
	if size <= 0 || size > qr.MaxSize {
		return fail(errout, 2, fmt.Errorf("--size 必须在 1—%d 之间", qr.MaxSize))
	}
	if mode != "half" && mode != "full" {
		return fail(errout, 2, fmt.Errorf("--terminal 必须为 half 或 full"))
	}
	level = qr.NormalizeLevel(level)
	if level != "L" && level != "M" && level != "Q" && level != "H" {
		return fail(errout, 2, fmt.Errorf("--level 必须为 L、M、Q 或 H"))
	}
	if provided["out"] && output == "" {
		return fail(errout, 2, fmt.Errorf("--out 的路径不能为空"))
	}
	if provided["file"] {
		var err error
		text, err = qr.ReadText(file)
		if err != nil {
			return fail(errout, 1, err)
		}
	}
	code, err := qr.New(text, level)
	if err != nil {
		return fail(errout, 1, err)
	}
	if output != "" {
		data, err := code.PNG(size)
		if err == nil {
			err = qr.SavePNG(output, data)
		}
		if err != nil {
			return fail(errout, 1, err)
		}
		return 0
	}
	if err := writeTerminal(out, errout, code.Modules, mode); err != nil {
		return fail(errout, 1, err)
	}
	return 0
}

// knownOption 识别当前子命令的选项名，用于检查字符串参数缺值。
func knownOption(fs *flag.FlagSet, value string) bool {
	if len(value) == 0 || value[0] != '-' {
		return false
	}
	name := value[1:]
	if len(name) > 0 && name[0] == '-' {
		name = name[1:]
	}
	name, _, _ = strings.Cut(name, "=")
	return fs.Lookup(name) != nil || name == "help" || name == "h"
}

// fail 输出错误并返回指定退出码。
func fail(errout io.Writer, code int, err error) int {
	fmt.Fprintln(errout, "错误：", err)
	return code
}

// writeTerminal 按终端宽度输出二维码，重定向时保留 ANSI 控制序列。
func writeTerminal(out, errout io.Writer, modules [][]bool, mode string) error {
	if file, ok := out.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		if os.Getenv("TERM") == "dumb" {
			return fmt.Errorf("当前终端不支持黑白颜色输出，请通过 --out 保存 PNG")
		}
		width, _, err := term.GetSize(int(file.Fd()))
		if err != nil {
			fmt.Fprintln(errout, "提示：无法读取终端宽度，将完整输出；如有折行，请调宽窗口后重试：", err)
		} else if err := checkWidth(len(modules), mode, width); err != nil {
			return err
		}
		return writeConsole(file, modules, mode)
	}
	_, err := io.WriteString(out, renderTerminal(modules, mode))
	return err
}

// checkWidth 为最右列自动换行留一个字符格；width 是当前窗格的字符列数。
func checkWidth(modules int, mode string, width int) error {
	needed := modules + 1
	if mode == "full" {
		needed = modules*2 + 1
	}
	if width < needed {
		return fmt.Errorf("终端至少需要 %d 列（含一列余量），当前 %d 列；请调宽后重试或通过 --out 保存 PNG", needed, width)
	}
	return nil
}
