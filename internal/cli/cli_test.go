package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"qrplain/internal/qr"
)

// TestCLIWorkflow 验证文件输入、实际 PNG 文件、识别标准输出以及不覆盖行为。
func TestCLIWorkflow(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "正文 文件.txt")
	output := filepath.Join(dir, "二维码 图片.png")
	content := " 中文🙂 \r\n\n末尾\n"
	os.WriteFile(input, []byte(content), 0600)
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"encode", "--file", input, "--out", output}, &stdout, &stderr); code != 0 || stdout.Len() != 0 {
		t.Fatalf("生成失败：%d %s", code, &stderr)
	}
	if code := Run([]string{"decode", "--image", output}, &stdout, &stderr); code != 0 || stdout.String() != content+"\n" {
		t.Fatalf("识别不一致：%d %q %s", code, &stdout, &stderr)
	}
	if code := Run([]string{"encode", "--text", "替换", "--out", output}, &stdout, &stderr); code != 1 {
		t.Fatal("已有文件未拒绝")
	}
}

// TestArgumentErrors 检查缺值、互斥和未知参数的退出码及输出。
func TestArgumentErrors(t *testing.T) {
	cases := [][]string{
		nil, {"unknown"}, {"encode"}, {"encode", "--text"},
		{"encode", "--text", ""}, {"encode", "--file", ""},
		{"encode", "--text", "--out"}, {"encode", "--file", "--text", "x"},
		{"encode", "--text", "--out=qr.png"},
		{"encode", "--text", "x", "--file", "a"},
		{"encode", "--text", "x", "--size", "0"},
		{"encode", "--text", "x", "--size", "4097"},
		{"encode", "--text", "x", "--level", "Z"},
		{"encode", "--text", "x", "--terminal", "other"},
		{"encode", "--text", "x", "--out", ""},
		{"encode", "--text", "x", "extra"}, {"decode"},
		{"decode", "--image"}, {"decode", "--unknown"},
	}
	for _, args := range cases {
		var stdout, stderr bytes.Buffer
		if code := Run(args, &stdout, &stderr); code != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("参数 %q 的退出码或输出不正确：%d %q %q", args, code, &stdout, &stderr)
		}
	}
}

// TestProcessingFailures 检查文件、编码和输出错误的退出码，以及帮助和等号传值。
func TestProcessingFailures(t *testing.T) {
	for _, args := range [][]string{
		{"encode", "--file", filepath.Join(t.TempDir(), "missing")},
		{"decode", "--image", filepath.Join(t.TempDir(), "missing")},
		{"encode", "--text", strings.Repeat("a", 3000)},
	} {
		var stdout, stderr bytes.Buffer
		if code := Run(args, &stdout, &stderr); code != 1 || stdout.Len() != 0 {
			t.Fatalf("处理错误退出码不正确：%q %d", args, code)
		}
	}
	var stdout, stderr bytes.Buffer
	if Run([]string{"encode", "--help"}, &stdout, &stderr) != 0 {
		t.Fatal("帮助命令失败")
	}
	if Run([]string{"encode", "--text", "x"}, brokenWriter{}, &stderr) != 1 {
		t.Fatal("输出写入失败未上报")
	}
	stdout.Reset()
	if Run([]string{"encode", "--text=--out"}, &stdout, &stderr) != 0 || stdout.Len() == 0 {
		t.Fatal("等号传入选项名称作为正文失败")
	}
}

// brokenWriter 模拟输出设备写入失败。
type brokenWriter struct{}

// Write 始终返回写入失败，不接收任何字节。
func (brokenWriter) Write(p []byte) (int, error) { return 0, errors.New("模拟输出失败") }

// TestTerminalMatrix 从半块输出还原每个模块，验证留白、颜色与最后一行补白。
func TestTerminalMatrix(t *testing.T) {
	code, err := qr.New("终端输出🙂", "M")
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if exit := Run([]string{"encode", "--text", "终端输出🙂"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("终端输出失败：%s", &stderr)
	}
	output := stdout.String()
	if !strings.Contains(output, "\x1b[30;47m") {
		t.Fatal("未明确设置黑白颜色")
	}
	plain := regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(output, "")
	lines := strings.Split(strings.TrimSuffix(plain, "\n"), "\n")
	if len(lines) != (len(code.Modules)+1)/2 {
		t.Fatal("输出行数不正确")
	}
	for line, value := range lines {
		row := []rune(value)
		if len(row) != len(code.Modules) {
			t.Fatal("二维码被裁切或加入其他字符")
		}
		for x, cell := range row {
			upper := cell == '█' || cell == '▀'
			lower := cell == '█' || cell == '▄'
			if upper != code.Modules[line*2][x] {
				t.Fatal("上半模块改变")
			}
			if line*2+1 < len(code.Modules) && lower != code.Modules[line*2+1][x] {
				t.Fatal("下半模块改变")
			}
		}
	}
	if checkWidth(185, "half", 185) == nil || checkWidth(185, "half", 186) != nil || checkWidth(185, "full", 370) == nil || checkWidth(185, "full", 371) != nil {
		t.Fatal("终端宽度或防折行余量不正确")
	}
	full := renderTerminal(code.Modules, "full")
	full = regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(full, "")
	for _, line := range strings.Split(strings.TrimSuffix(full, "\n"), "\n") {
		if len(line) != len(code.Modules)*2 {
			t.Fatal("全块模式宽度不正确")
		}
	}
}
