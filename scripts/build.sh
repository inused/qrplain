#!/bin/sh
# 使用当前 Go 工具链构建六种平台产物，输出到 dist/；任一目标失败即停止。
# 用法：在项目根目录执行 sh scripts/build.sh。
set -eu

mkdir -p dist/docs
# 许可文件随可执行文件分发。
cp LICENSE dist/LICENSE
cp docs/THIRD_PARTY.md dist/docs/THIRD_PARTY.md
for target in windows/amd64 windows/arm64 linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
    target_os="${target%/*}"
    target_arch="${target#*/}"
    extension=""
    if [ "$target_os" = windows ]; then
        extension=".exe"
    fi
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -trimpath -ldflags '-s -w' -o "dist/qrplain-$target_os-$target_arch$extension" .
done
