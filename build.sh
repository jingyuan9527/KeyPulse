#!/usr/bin/env bash
# KeyPulse 构建脚本（Windows / Git Bash）
#
# 用法：
#   bash build.sh                        # 编译到项目根目录（日常开发）
#   bash build.sh dist/KeyPulse-<版本>   # 编译到指定目录（发布打包，可绕开被占用的 exe）
#
# 默认使用 PATH 里的 go。便携版 Go 或需要指定 GOROOT/GOCACHE/代理时，
# 复制 .env.local.example 为 .env.local 并按需填写；该文件不纳入版本库。
set -e

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

if [ -f "$ROOT/.env.local" ]; then
  . "$ROOT/.env.local"
fi

if [ -n "${KP_GO_BIN:-}" ]; then export PATH="$KP_GO_BIN:$PATH"; fi
if [ -n "${KP_GOROOT:-}" ]; then export GOROOT="$KP_GOROOT"; fi
if [ -n "${KP_GOCACHE:-}" ]; then export GOCACHE="$KP_GOCACHE"; fi
export GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}"

if ! command -v go >/dev/null 2>&1; then
  echo "错误：未找到 go。请把 Go 1.25+ 加入 PATH，或配置 .env.local 的 KP_GO_BIN。" >&2
  exit 1
fi

export GOOS=windows GOARCH=amd64 CGO_ENABLED=0

VERSION="$(tr -d '\r\n' < VERSION)"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-s -w -X keypulse/internal/version.Version=${VERSION} -X keypulse/internal/version.BuildTime=${BUILD_TIME}"

OUT_DIR="${1:-.}"
mkdir -p "$OUT_DIR"

# -buildvcs=false：产物不嵌 VCS 状态，便于复现；非 git 目录下也能构建。
# 日常版无控制台窗口，托盘常驻；调试版保留控制台用于 --version / --selftest。
go build -buildvcs=false -trimpath -ldflags="${LDFLAGS} -H=windowsgui" -o "${OUT_DIR}/keypulse.exe" ./cmd/keypulse
go build -buildvcs=false -trimpath -ldflags="${LDFLAGS}" -o "${OUT_DIR}/keypulse-cli.exe" ./cmd/keypulse

echo "构建完成 ${VERSION} (${BUILD_TIME}): ${OUT_DIR}/keypulse.exe / keypulse-cli.exe"
