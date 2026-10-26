package qr

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRoundTrip 验证多语言、CRLF 与首尾空白经过独立编解码器后保持原样。
func TestRoundTrip(t *testing.T) {
	for _, content := range []string{"hello", " 中文与 Emoji🙂\r\n\r\n第二行 \n", strings.Repeat("1234567890", 250) + "x"} {
		code, err := New(content, "M")
		if err != nil {
			t.Fatal(err)
		}
		data, err := code.PNG(1024)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Decode(bytes.NewReader(data))
		if err != nil || got != content {
			t.Fatalf("正文不一致：got=%q want=%q error=%v", got, content, err)
		}
	}
}

// TestCapacity 检查四种纠错等级的三类标准容量及各自超出一单位的拒绝路径。
func TestCapacity(t *testing.T) {
	cases := []struct {
		level                  string
		numeric, alpha, binary int
	}{{"L", 7089, 4296, 2953}, {"M", 5596, 3391, 2331}, {"Q", 3993, 2420, 1663}, {"H", 3057, 1852, 1273}}
	for _, tc := range cases {
		for _, content := range []struct {
			name, character string
			limit           int
		}{{"数字", "1", tc.numeric}, {"字母数字", "A", tc.alpha}, {"字节", "a", tc.binary}} {
			t.Run(tc.level+"/"+content.name, func(t *testing.T) {
				text := strings.Repeat(content.character, content.limit)
				code, err := New(text, tc.level)
				if err != nil || code.Version != 40 {
					t.Fatalf("未达到 Version 40 标准边界：%v", err)
				}
				if _, err := New(text+content.character, tc.level); err == nil {
					t.Fatal("超出标准容量仍然成功")
				}
				data, err := code.PNG(1024)
				if err != nil {
					t.Fatal(err)
				}
				got, err := Decode(bytes.NewReader(data))
				if err != nil || got != text {
					t.Fatalf("容量边界回读不一致：%v", err)
				}
			})
		}
	}
}

// TestMixedMode 检查长数字与小写字母的混合分段编码。
func TestMixedMode(t *testing.T) {
	if _, err := New(strings.Repeat("1", 4000)+"x", "M"); err != nil {
		t.Fatalf("分段优化失败：%v", err)
	}
}

// TestImageDimensions 检查 PNG 实际尺寸、模块像素宽度和非法尺寸错误。
func TestImageDimensions(t *testing.T) {
	code, err := New("尺寸检查", "M")
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{-1, 0, len(code.Modules) - 1, MaxSize + 1} {
		if _, err := code.PNG(size); err == nil {
			t.Fatalf("无效尺寸 %d 未拒绝", size)
		}
	}
	data, err := code.PNG(513)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil || img.Bounds().Dx() != 513 || img.Bounds().Dy() != 513 {
		t.Fatalf("图片尺寸错误：%v", err)
	}
	scale := 513 / len(code.Modules)
	padding := (513 - len(code.Modules)*scale) / 2
	for y, row := range code.Modules {
		for x, black := range row {
			for _, offset := range []int{0, scale - 1} {
				r, _, _, _ := img.At(padding+x*scale+offset, padding+y*scale+offset).RGBA()
				if (r == 0) != black {
					t.Fatal("模块未按整数像素绘制")
				}
			}
		}
	}
}

// TestImageVariants 覆盖有损 JPEG、旋转、透明背景与空白图片识别。
func TestImageVariants(t *testing.T) {
	code, _ := New("中文 JPEG 和透明图片🙂", "M")
	data, _ := code.PNG(512)
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	rotated := image.NewRGBA(img.Bounds())
	transparent := image.NewNRGBA(img.Bounds())
	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			rotated.Set(511-y, x, img.At(x, y))
			r, _, _, _ := img.At(x, y).RGBA()
			if r == 0 {
				transparent.Set(x, y, color.Black)
			}
		}
	}
	for _, variant := range []image.Image{img, rotated, transparent} {
		var b bytes.Buffer
		if variant == img {
			err = jpeg.Encode(&b, variant, &jpeg.Options{Quality: 85})
		} else {
			err = png.Encode(&b, variant)
		}
		if err != nil {
			t.Fatal(err)
		}
		got, err := Decode(&b)
		if err != nil || got != "中文 JPEG 和透明图片🙂" {
			t.Fatalf("图片变体识别失败：%q %v", got, err)
		}
	}
	var blank bytes.Buffer
	png.Encode(&blank, image.NewGray(image.Rect(0, 0, 32, 32)))
	if _, err := Decode(&blank); err == nil {
		t.Fatal("空白图片被报告为成功")
	}
}

// TestInvalidInput 覆盖错误编码、损坏图片、文件长度及解码前的像素预算校验。
func TestInvalidInput(t *testing.T) {
	for _, tc := range []struct{ text, level string }{{"", "M"}, {"\xff", "M"}, {"x", "Z"}} {
		if _, err := New(tc.text, tc.level); err == nil {
			t.Fatal("无效正文或等级未拒绝")
		}
	}
	for _, data := range [][]byte{nil, []byte("not a png"), make([]byte, MaxImageBytes+1)} {
		if _, err := Decode(bytes.NewReader(data)); err == nil {
			t.Fatal("无效图片未拒绝")
		}
	}
	// 构造合法 IHDR 校验和但超出像素预算的头部，验证分配像素内存前已拒绝。
	var encoded bytes.Buffer
	png.Encode(&encoded, image.NewGray(image.Rect(0, 0, 1, 1)))
	header := encoded.Bytes()
	binary.BigEndian.PutUint32(header[16:20], 100000)
	binary.BigEndian.PutUint32(header[20:24], 100000)
	binary.BigEndian.PutUint32(header[29:33], crc32.ChecksumIEEE(header[12:29]))
	if _, err := Decode(bytes.NewReader(header)); err == nil || !strings.Contains(err.Error(), "像素") {
		t.Fatalf("未在解码前拒绝超大图片：%v", err)
	}
}

// TestFiles 验证 UTF-8 BOM、保留换行、文件缺失及不覆盖已有输出文件。
func TestFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "中文 正文.txt")
	content := " \r\n中文\n\n"
	if err := os.WriteFile(path, []byte("\xef\xbb\xbf"+content), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadText(path)
	if err != nil || got != content {
		t.Fatalf("文件正文改变：%q %v", got, err)
	}
	for _, invalid := range [][]byte{{0xff}, bytes.Repeat([]byte{'1'}, MaxTextBytes+1)} {
		os.WriteFile(path, invalid, 0600)
		if _, err := ReadText(path); err == nil {
			t.Fatal("无效文件被接受")
		}
	}
	if _, err := ReadText(filepath.Join(dir, "缺失")); err == nil {
		t.Fatal("缺失文件被接受")
	}
	output := filepath.Join(dir, "结果.png")
	if err := SavePNG(output, []byte("original")); err != nil {
		t.Fatal(err)
	}
	if err := SavePNG(output, []byte("replacement")); err == nil {
		t.Fatal("已有文件被覆盖")
	}
	data, _ := os.ReadFile(output)
	if string(data) != "original" {
		t.Fatal("原文件内容改变")
	}
	if err := SavePNG(filepath.Join(dir, "不存在", "qr.png"), nil); err == nil {
		t.Fatal("不存在的输出目录未报错")
	}
}
