# QRPlain · 本地二维码

QRPlain 是本地离线二维码生成与解析工具，提供浏览器界面和 CLI。程序支持 UTF-8 文本、PNG 保存和 PNG/JPEG 图片识别，页面资源随可执行文件内嵌。

构建目标为 Windows、Linux、macOS 的 amd64 和 arm64。Windows 7 的兼容目标为 amd64，Linux 重点验证环境为银河麒麟 V10。实际验证范围见 [验证记录](docs/VALIDATION.md)。

## 浏览器界面

运行对应平台的可执行文件，或执行：

```sh
qrplain ui
```

程序打开本机浏览器。自动打开失败时，手动访问终端显示的完整地址。服务监听 `127.0.0.1`，地址中的访问凭据在本次运行期间有效。

在“内容”区域输入正文，设置尺寸和纠错等级，点击“生成二维码”，再用“保存生成的 PNG”下载图片。文件位置和重名处理由浏览器决定。修改正文或参数后，需要重新生成。

“打开图片识别”接受 PNG、JPEG，结果显示在同一个内容区域。识别结果未编辑就重新生成时，程序保留原始换行。

点击“退出程序”或在终端按 Ctrl+C 结束服务；关闭标签页会保留进程。无参数运行等同于 `ui`。Windows 双击启动时可能显示控制台窗口。

## CLI

以下示例使用命令名 `qrplain`，也可替换为实际文件路径，例如 `./dist/qrplain-linux-amd64`。Windows 使用对应的 `.exe` 文件。

```sh
# 在终端显示二维码。
qrplain encode --text "你好，二维码"

# 读取 UTF-8 文件。
qrplain encode --file content.txt

# 保存 PNG。
qrplain encode --file content.txt --out qr.png --size 1024 --level M

# 识别图片。
qrplain decode --image qr.png

qrplain --help
qrplain --version
```

| 选项 | 含义 |
| --- | --- |
| `--text` | 正文；与 `--file` 互斥，缺值或空值报错 |
| `--file` | UTF-8 文件路径；去除开头 BOM，保留其余空白、CRLF 和末尾换行 |
| `--out` | PNG 保存路径；父目录须存在，已有文件报错 |
| `--size` | PNG 边长，默认 512、最大 4096 像素；须容纳全部模块和留白 |
| `--level` | 纠错等级 L/M/Q/H，默认 M；等级越高，通常容量越小 |
| `--terminal` | `half` 为默认半块模式；`full` 每模块占两列，适合不支持半块字符的终端 |
| `decode --image` | PNG/JPEG 图片路径 |

选项放在子命令后。正文恰好是已知选项名时，用等号传值，例如 `--text=--out`。

`encode` 默认显示终端二维码，指定 `--out` 时保存 PNG。`decode` 将完整正文写入标准输出，再补一个结果结束换行。错误和提示写入标准错误。

退出码：`0` 成功，`1` 处理或输出失败，`2` 参数错误。

### 终端显示

默认使用 Unicode 半块字符与黑白颜色。旧 Windows 控制台可使用 `--terminal full`，通过原生控制台 API 设置颜色。`--size` 只控制 PNG 像素边长。

列数是当前窗口或窗格每行容纳的字符格数。拉宽窗口或缩小字体可以增加列数；SSH、tmux 以当前会话或窗格的宽度为准。

输出前会检查宽度，并为最右列自动换行预留一列。最大版本半块模式需要至少 186 列，全块模式需要至少 371 列。宽度不足时，按提示调宽后重试，或用 `--out` 保存 PNG。

读取不到终端尺寸时，程序提示后完整输出。重定向的内容包含 ANSI 控制序列。调整窗口后请重新执行；手机扫码时，整个二维码需要同时可见。

## 容量与图片尺寸

普通二维码最大为 Version 40。下表是单一编码模式的标准容量：

| 模式 | L | M | Q | H |
| --- | ---: | ---: | ---: | ---: |
| 数字 | 7089 位 | 5596 位 | 3993 位 | 3057 位 |
| 标准字母数字字符集 | 4296 字符 | 3391 字符 | 2420 字符 | 1852 字符 |
| 字节 | 2953 字节 | 2331 字节 | 1663 字节 | 1273 字节 |

标准字母数字字符集包含数字、大写英文字母及部分符号。常见汉字通常占 3 个 UTF-8 字节，L 级约可容纳 984 个常见汉字。混合正文的实际容量由编码方式决定；生成器使用混合分段编码减少占用，超限时报错。

PNG 的每个模块使用等宽整数像素，四周保留白边。密集二维码建议使用 768 或 1024 像素，过小的图片会影响扫描稳定性。

图片识别支持单个二维码。输入格式为 PNG/JPEG，文件大小上限为 16 MiB，像素数上限为 16,777,216。

## 构建与测试

使用 Go 1.20.7，依赖版本记录在 `go.mod` 和 `go.sum`：

```sh
go test ./...
go vet ./...
go build -trimpath -ldflags "-s -w" -o dist/qrplain .
```

Windows 构建命令：

```bat
go build -trimpath -ldflags "-s -w" -o dist/qrplain.exe .
```

默认版本号为 `dev`，可通过链接参数 `-X main.version=版本号` 指定。

在项目根目录构建六种平台产物：

```sh
sh scripts/build.sh
```

Windows CMD 使用：

```bat
scripts\build.bat
```

两个脚本均使用 `CGO_ENABLED=0`，任一目标失败即停止。BAT 按自身位置定位项目根目录，执行后恢复工作目录和环境变量，也可从其他目录通过完整路径调用。

输出在 `dist/`，其中 `LICENSE` 和 `docs/THIRD_PARTY.md` 需随可执行文件分发。Linux 下载文件可能需要添加执行权限。

### 浏览器端到端测试

使用已有 Chromium 和 Node.js 22 或更新版本：

```sh
QR_TEST_BROWSER=/path/to/chrome node scripts/browser-test.mjs
```

脚本运行 `dist/qrplain-linux-amd64`，验证生成、下载、上传识别、旧图失效、换行保真、图片尺寸、窄屏布局和退出。测试图片及截图保存在 `.cache/browser-test/`。

## 开源许可

原创代码采用 [MIT 许可证](LICENSE)。第三方组件遵循各自许可证，许可与附加专利授权原文见 [第三方许可说明](docs/THIRD_PARTY.md)。

技术资料：[技术设计](docs/PLAN.md)、[验证记录](docs/VALIDATION.md)。
