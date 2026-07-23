# Stage 1: 构建前端
FROM node:22-alpine AS frontend-builder
WORKDIR /app/frontend
# 锁定 pnpm 10：pnpm 11 对 esbuild 的 build scripts 直接报错（ERR_PNPM_IGNORED_BUILDS）
RUN corepack enable && corepack prepare pnpm@10.34.5 --activate
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm build

# Stage 2: 构建后端（静态编译）
FROM golang:1.26-alpine AS backend-builder
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /email-notify-server ./cmd/server

# Stage 3: 运行时
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=backend-builder /email-notify-server /usr/local/bin/email-notify-server
COPY --from=frontend-builder /app/frontend/dist /app/dist
ENV LISTEN_ADDR=0.0.0.0:8000 \
    CONFIG_DIR=/data \
    STATIC_DIR=/app/dist
RUN mkdir -p /data
VOLUME /data
EXPOSE 8000
HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
  CMD wget -q --spider http://localhost:8000/api/health || exit 1
ENTRYPOINT ["email-notify-server"]
