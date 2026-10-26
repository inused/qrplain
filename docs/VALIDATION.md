# QRPlain 验证记录

## 环境

验证日期为 2026-10-26。Go 测试和构建环境为 Linux amd64、Go 1.20.7；页面测试使用 Chromium 无头模式和 `scripts/browser-test.mjs`。

## 平台与运行验证

| 平台 | 纯 Go 构建 | 运行验证 |
| --- | --- | --- |
| Windows amd64 | 通过 | 待实机验证，含 Windows 7 |
| Windows arm64 | 通过 | 待实机验证 |
| Linux amd64 | 通过 | 本机 CLI、伪终端和 Chromium 页面通过 |
| Linux arm64 | 通过 | 待实机验证 |
| macOS amd64 | 通过 | 待实机验证 |
| macOS arm64 | 通过 | 待实机验证 |

麒麟 V10 的 amd64、arm64，Windows 旧控制台，Firefox 和真实扫码器均待对应环境验证。Linux 两种产物均为静态链接 ELF。

## 自动化检查

| 检查 | 结果 |
| --- | --- |
| `go test ./...` | 通过 |
| `go vet ./...` | 通过 |
| `go test -race ./...` | 通过 |
| 容量边界 | L/M/Q/H 的数字、字母数字、字节模式共 12 组最大容量生成与回读通过，超出一单位时报错 |
| 混合分段 | 4000 位数字追加小写字符可在 M 级编码；混合样本回读一致 |
| 文本保真 | 中文、英文、Emoji、空白、CRLF、末尾换行及 UTF-8 BOM 文件通过 |
| 图片尺寸 | PNG 实际边长和模块像素宽度符合请求，非法尺寸报错 |
| 图片识别 | PNG、JPEG、旋转和透明背景通过；损坏、空白、超文件大小及像素预算的图片报错 |
| 文件与输出错误 | 缺失文件、无效 UTF-8、已有输出文件、缺失父目录和写入失败均返回错误 |
| CLI 参数 | 缺值、空值、互斥、未知参数和等号传值的退出码及输出通过 |
| 终端矩阵 | 半块输出还原后与生成矩阵一致；全块宽度和一列余量检查通过 |
| 本机 API | Host、Origin、凭据、请求方法、并发任务和请求内容检查通过 |
| Chromium 端到端 | 生成、下载 `qrplain.png`、上传识别、旧图失效、CRLF 保真、尺寸、窄屏布局和退出通过 |
| 页面资源 | 内嵌文件可读，凭据隔离及页面响应头检查通过 |
| JavaScript | `node --check` 通过 |
| 许可 | 项目许可证和第三方许可原文完整，分发副本与源文件一致 |

## 终端与脚本

Linux 伪终端在清除 DISPLAY、WAYLAND_DISPLAY 后运行最大 L 级数字码：

| 模式 | 列数 | 结果 |
| --- | ---: | --- |
| half | 80、185 | 提示宽度不足 |
| half | 186 | 完整输出 185 列、93 行 |
| full | 370 | 提示宽度不足 |
| full | 371 | 完整输出 370 列、185 行 |

`scripts/build.sh` 完成六种产物编译并复制许可文件。`scripts/build.bat` 的目标矩阵、参数、输出名、失败退出、环境恢复和 CRLF 行尾已静态核对，Windows CMD 执行待验证。

实际终端中的字体、主题、SSH/tmux 窗格和窗口缩放，以及手机对密集二维码的扫描效果，需在对应环境验证。

## 产物体积

构建参数为 `CGO_ENABLED=0`、`-trimpath`、`-ldflags '-s -w'`，版本为 dev。下表记录未压缩可执行文件大小：

| 产物 | 字节 |
| --- | ---: |
| qrplain-windows-amd64.exe | 6734336 |
| qrplain-windows-arm64.exe | 6323200 |
| qrplain-linux-amd64 | 6479872 |
| qrplain-linux-arm64 | 6160384 |
| qrplain-darwin-amd64 | 6910448 |
| qrplain-darwin-arm64 | 6737218 |
