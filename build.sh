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
# 个别 CI 环境 npm 会安装出空壳包，指定全新缓存目录；失败则切换 corepack pnpm
npm ci --include=dev --cache /tmp/npm-build-cache --no-audit --no-fund || true
if [ ! -e node_modules/vite/bin/vite.js ]; then
  echo "npm 安装异常（vite 文件缺失），切换 corepack pnpm"
  rm -rf node_modules
  corepack enable || true
  if [ -f pnpm-lock.yaml ]; then
    corepack pnpm@10 install --frozen-lockfile
  else
    corepack pnpm@10 install
  fi
fi
# 个别 CI 环境 npm 不生成 node_modules/.bin，兜底直接调用 vite.js
if [ -x node_modules/.bin/vite ]; then
  npm run build
else
  node node_modules/vite/bin/vite.js build
fi
cp -r "$ROOT/frontend/dist" "$OUT/dist"

echo "==> 构建后端"
cd "$ROOT/backend"
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$OUT/email-notify-server" ./cmd/server

echo "==> 完成: $OUT"
ls -la "$OUT"
