# 邮件提醒器（lzc-email-notify）

运行在懒猫微服（LazyCat Cloud / lzcos）上的邮箱新邮件提醒应用。

通过 **IMAP / POP3** 定时巡检多个邮箱（最小检查间隔 1 分钟），发现新邮件时调用懒猫微服的客户端通知能力推送系统通知，**点击通知直接跳转到对应邮箱的网页版页面**。

## 功能

- 多邮箱监控：支持同时配置多个 IMAP / POP3 邮箱
- 检查间隔最小 1 分钟，每个账号独立轮询
- 新邮件系统通知（标题为账号名，正文为发件人与主题），点击通知跳转该邮箱的 Web 版地址
- 首次巡检只建立基线，不会对历史邮件轰炸通知
- 账号连接测试、手动立即检查、启用/禁用开关
- 最近事件流（新邮件、通知失败、巡检失败等）
- 多用户隔离：按懒猫登录用户（X-HC-User-ID）隔离各自账号配置

## 技术栈

- 后端：Go（静态编译单二进制，go-imap/v2 + 手写最小 POP3 客户端 + lzc-sdk 通知）
- 前端：React (JavaScript) + Vite + Tailwind CSS + shadcn/ui 风格组件（HashRouter）
- 存储：JSON 文件（`/lzcapp/var/config.json`，平台持久化目录）

## 目录结构

```
├── Dockerfile           # 全量镜像构建（前端 + 后端 + 运行时，单容器）
├── .dockerignore
├── build.sh             # 全量构建脚本（前端 + 后端 → build-out/，CI 校验用）
├── start.sh             # contentdir 部署时代的启动脚本（已被 Docker 部署取代，保留兼容）
├── docs/api.md          # 前后端 API 契约
├── backend/             # Go 后端（cmd/server + internal/*）
└── frontend/            # React 前端（Vite）
```

> LPK 打包配置（`package.yml` / `lzc-manifest.yml` / `lzc-build*.yml` / `icon.png`）
> 统一维护在 `lzc-appdb` 仓库的 `lzc-email-notify/` 目录下，不在本仓库。

## 本地开发

```bash
# 后端（需 Go 1.26+）
cd backend
DEV_NOAUTH=1 CONFIG_DIR=./data go run ./cmd/server
# DEV_NOAUTH=1 时无 X-HC-User-ID 头按 dev-user 处理，方便本地调试

# 前端（需 Node 20+）
cd frontend
npm install
npm run dev
```

非懒猫环境下，通知发送会自动回落为日志输出（LogSender），其余功能不受影响。

## 构建与部署到懒猫微服

LPK 打包配置在 `lzc-appdb` 仓库的 `lzc-email-notify/` 目录（该目录下的
`build.sh` 是包装脚本，会调用本仓库的 `build.sh` 再把产物复制过去）：

```bash
npm install -g @lazycatcloud/lzc-cli
lzc-cli box list && lzc-cli box switch <你的微服>

cd ~/workspace/lzc-appdb/lzc-email-notify
lzc-cli project deploy        # 开发态部署（读 lzc-build.dev.yml）
lzc-cli project build         # 产出发布包（读 lzc-build.yml）
lzc-cli app install           # 安装到微服
```

## 说明

- 密码仅保存在微服本机 `/lzcapp/var/config.json`（权限 0600），不会经任何接口返回
- QQ/163 等邮箱请使用「授权码」而非登录密码
- 应用声明了 `background_task: true`，平台不会对其主动休眠，以保证持续巡检

## CI

`.github/workflows/build.yml`：前端构建 + 后端 vet/test/build + build.sh 全量构建与产物校验（LPK 打包配置在 `lzc-appdb` 仓库，CI 不再打包 LPK）。
