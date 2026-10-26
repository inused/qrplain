// Package qr 提供界面与 CLI 共用的 UTF-8 文本、二维码及 PNG/JPEG 处理。
package qr

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg" // 注册标准 JPEG 图片解码器。
	"image/png"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/makiuchi-d/gozxing"
	zxingqr "github.com/makiuchi-d/gozxing/qrcode"
	encoder "github.com/skip2/go-qrcode"
)

const (
	// DefaultSize 是生成 PNG 的默认边长，单位为像素。
	DefaultSize = 512
	// MaxSize 是生成 PNG 的最大边长，单位为像素，用于限制内存分配。
	MaxSize = 4096
	// MaxTextBytes 是支持模式的单码最大正文长度，单位为 UTF-8 字节。
	// 数字模式可达 7089 字节；其他内容按编码模式和纠错等级判断。
	MaxTextBytes = 7089
	// MaxImageBytes 是输入图片的文件大小上限，单位为字节。
	MaxImageBytes = 16 << 20
	// MaxImagePixels 是输入图片的像素数上限，防止压缩图片展开后占用过多内存。
	MaxImagePixels = 16 << 20
)

// Code 保存含标准留白的模块矩阵；Modules[y][x] 的 true 为黑色、false 为白色，Version 为 1—40。
type Code struct {
	Modules [][]bool
	Version int
}

// New 根据原文与纠错等级 L/M/Q/H 编码；空正文、无效 UTF-8 或容量不足时返回错误。
//
//	@param text string 正文，保留空白和换行。
//	@param level string 纠错等级，仅支持 L、M、Q、H。
//	@return *Code 含四模块留白的二维码；失败时为 nil。
//	@return error 输入无效或超过所选等级容量时的原因。
func New(text, level string) (*Code, error) {
	if text == "" {
		return nil, fmt.Errorf("正文不能为空")
	}
	if !utf8.ValidString(text) {
		return nil, fmt.Errorf("正文不是合法 UTF-8")
	}
	if len(text) > MaxTextBytes {
		return nil, fmt.Errorf("正文为 %d 字节，超过单码支持的最大长度 %d 字节", len(text), MaxTextBytes)
	}
	levels := map[string]encoder.RecoveryLevel{
		"L": encoder.Low, "M": encoder.Medium, "Q": encoder.High, "H": encoder.Highest,
	}
	ecc, ok := levels[level]
	if !ok {
		return nil, fmt.Errorf("纠错等级必须为 L、M、Q 或 H")
	}
	code, err := encoder.New(text, ecc)
	if err != nil {
		return nil, fmt.Errorf("无法生成二维码，正文可能超过 %s 级容量：%w", level, err)
	}
	return &Code{Modules: code.Bitmap(), Version: code.VersionNumber}, nil
}

// PNG 按指定边长绘制等宽整数像素模块，剩余空间填白。
//
//	@param size int PNG 的宽高像素数，必须容纳全部模块且不超过 MaxSize。
//	@return []byte 编码后的 PNG 文件内容；失败时为 nil。
//	@return error 尺寸无效或 PNG 编码失败时的原因。
func (c *Code) PNG(size int) ([]byte, error) {
	width := len(c.Modules)
	if size < width || size > MaxSize {
		return nil, fmt.Errorf("图片边长必须在 %d—%d 像素之间，当前为 %d", width, MaxSize, size)
	}
	img := image.NewPaletted(image.Rect(0, 0, size, size), color.Palette{color.White, color.Black})
	scale := size / width
	padding := (size - width*scale) / 2
	for y, row := range c.Modules {
		for x, black := range row {
			if !black {
				continue
			}
			for dy := 0; dy < scale; dy++ {
				start := img.PixOffset(padding+x*scale, padding+y*scale+dy)
				for dx := 0; dx < scale; dx++ {
					img.Pix[start+dx] = 1
				}
			}
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, img); err != nil {
		return nil, fmt.Errorf("编码 PNG 失败：%w", err)
	}
	return output.Bytes(), nil
}

// ReadText 读取 UTF-8 文件，去除开头 BOM；其余字节保持不变。
//
//	@param path string 正文文件路径。
//	@return string 文件正文；空文件返回空串。
//	@return error 打开、读取、UTF-8 校验或长度检查失败的原因。
func ReadText(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("打开文本文件失败：%w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxTextBytes+4))
	if err != nil {
		return "", fmt.Errorf("读取文本文件失败：%w", err)
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if len(data) > MaxTextBytes {
		return "", fmt.Errorf("文件正文超过单码支持的最大长度 %d 字节", MaxTextBytes)
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("文本文件不是合法 UTF-8，请先转换编码")
	}
	return string(data), nil
}

// Decode 读取 PNG/JPEG 并识别一个二维码；透明背景合成白底。
//
//	@param input io.Reader PNG 或 JPEG 图片内容；超过 MaxImageBytes 时返回错误。
//	@return string 识别正文，保留正文空白；失败时为空串。
//	@return error 格式不支持、图片过大或损坏、无可识别二维码、正文非 UTF-8 时的原因。
func Decode(input io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(input, MaxImageBytes+1))
	if err != nil {
		return "", fmt.Errorf("读取图片失败：%w", err)
	}
	if len(data) > MaxImageBytes {
		return "", fmt.Errorf("图片文件不能超过 16 MiB")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("读取 PNG/JPEG 图片信息失败：%w", err)
	}
	if format != "png" && format != "jpeg" {
		return "", fmt.Errorf("仅支持 PNG、JPEG 图片")
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > MaxImagePixels {
		return "", fmt.Errorf("图片像素数必须在 1—%d 之间", MaxImagePixels)
	}
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("解码图片失败：%w", err)
	}
	// 透明黑色像素需要合成白底，以免被识别为黑色模块。
	flat := image.NewRGBA(image.Rect(0, 0, config.Width, config.Height))
	draw.Draw(flat, flat.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), source, source.Bounds().Min, draw.Over)
	bitmap, err := gozxing.NewBinaryBitmapFromImage(flat)
	if err != nil {
		return "", fmt.Errorf("准备二维码图片失败：%w", err)
	}
	reader := zxingqr.NewQRCodeReader()
	result, err := reader.Decode(bitmap, map[gozxing.DecodeHintType]interface{}{
		gozxing.DecodeHintType_TRY_HARDER: true,
	})
	if err != nil {
		// 高版本的重复数据可能干扰定位，纯码模式适用于无透视变形的二维码。
		// 此模式执行二维码结构与纠错校验。
		result, err = reader.Decode(bitmap, map[gozxing.DecodeHintType]interface{}{
			gozxing.DecodeHintType_PURE_BARCODE: true,
		})
	}
	if err != nil {
		return "", fmt.Errorf("图片中没有可识别的二维码：%w", err)
	}
	text := result.GetText()
	if !utf8.ValidString(text) {
		return "", fmt.Errorf("二维码正文不是合法 UTF-8")
	}
	return text, nil
}

// SavePNG 新建 PNG 文件；已有文件报错，保存失败时清理未完成的文件。
//
//	@param path string 新 PNG 文件路径，父目录必须存在。
//	@param data []byte 已编码的 PNG 内容。
//	@return error 创建、写入、关闭或失败清理时的原因；成功为 nil。
func SavePNG(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("新建 PNG 文件失败，输出路径须为新文件：%w", err)
	}
	_, writeErr := io.Copy(f, bytes.NewReader(data))
	closeErr := f.Close()
	if writeErr == nil {
		writeErr = closeErr
	}
	if writeErr == nil {
		return nil
	}
	if cleanupErr := os.Remove(path); cleanupErr != nil {
		return fmt.Errorf("保存图片失败：%v；清理未完成文件失败：%w", writeErr, cleanupErr)
	}
	return fmt.Errorf("保存图片失败：%w", writeErr)
}

// NormalizeLevel 将纠错等级转为大写。
//
//	@param level string 用户输入的纠错等级。
//	@return string 大写等级；有效性由 New 检查。
func NormalizeLevel(level string) string {
	return strings.ToUpper(level)
}
