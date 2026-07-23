#!/bin/sh
# 按容器 CPU 架构选择后端二进制（amd64 / arm64）
arch=$(uname -m)
case "$arch" in
  x86_64|amd64)
    exec /lzcapp/pkg/content/email-notify-server-amd64
    ;;
  aarch64|arm64)
    exec /lzcapp/pkg/content/email-notify-server-arm64
    ;;
  *)
    echo "unsupported arch: $arch" >&2
    exit 1
    ;;
esac
