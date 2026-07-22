#!/bin/sh
# lzc-email-notify 全量构建脚本
# 产物归置到 build-out/（lzc-build.yml 的 contentdir）：
#   build-out/dist/               前端静态产物（file:// 路由）
#   build-out/email-notify-server 后端静态二进制（upstreams backend_launch_command）
set -e

ROOT=$(cd "$(dirname "$0")" && pwd)
OUT="$ROOT/build-out"

rm -rf "$OUT"
mkdir -p "$OUT"

echo "==> 构建前端"
cd "$ROOT/frontend"
if [ -f package-lock.json ]; then
  npm ci --include=dev
else
  npm install --include=dev
fi
npm run build
cp -r "$ROOT/frontend/dist" "$OUT/dist"

echo "==> 构建后端"
cd "$ROOT/backend"
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$OUT/email-notify-server" ./cmd/server

echo "==> 完成: $OUT"
ls -la "$OUT"
