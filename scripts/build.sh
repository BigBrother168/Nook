#!/usr/bin/env bash
# 构建 Nook：先编译门卫（nook-guard.exe）、拷许可证，都放进 resources 随程序发布，
# 再打包。出来的是 build/windows-amd64/：程序和它的文件，加上安装包
# nook-setup-<版本>-amd64.exe（中文界面、卸载问删不删设置，见 scripts/installer.nsh）。
set -euo pipefail
cd "$(dirname "$0")/.."
export CGO_ENABLED=0
go build -trimpath -ldflags "-s -w" -o resources/nook-guard.exe ./cmd/nook-guard
cp LICENSE resources/LICENSE.txt
go tool mygo build
