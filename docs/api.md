# lzc-email-notify 前后端 API 契约

本文件是前端与后端实现的**唯一事实源**。两端必须严格按此契约实现。

## 通用约定

- 所有 API 以 `/api` 为前缀，JSON 编解码，`Content-Type: application/json`
- 平台 lzc-ingress 会注入 `X-HC-User-ID` 请求头，后端直接信任它作为用户 uid；数据按 uid 隔离
- 错误响应统一为：`{"error": "人类可读的中文错误信息"}`，HTTP 状态码 4xx/5xx
- 成功响应不包裹额外 envelope，直接返回数据本体

## 数据模型

### Account（邮箱监控账号）

```json
{
  "id": "9f2c1a7e4b6d4f2e",
  "name": "QQ 邮箱",
  "protocol": "imap",
  "host": "imap.qq.com",
  "port": 993,
  "ssl": true,
  "username": "someone@qq.com",
  "has_password": true,
  "interval_sec": 60,
  "web_url": "https://mail.qq.com",
  "enabled": true,
  "created_at": "2026-07-22T14:00:00+08:00",
  "updated_at": "2026-07-22T14:00:00+08:00",
  "status": {
    "checking": false,
    "last_check_at": "2026-07-22T14:05:00+08:00",
    "last_success_at": "2026-07-22T14:05:00+08:00",
    "last_error": "",
    "baseline_done": true,
    "last_mail": {
      "from": "张三 <zhangsan@example.com>",
      "subject": "本周例会通知",
      "date": "2026-07-22T13:58:11+08:00"
    }
  }
}
```

字段规则：

- `protocol`：`"imap"` 或 `"pop3"`
- `ssl`：true = 隐式 TLS（IMAP 993 / POP3 995）；false = 明文连接，IMAP 下若服务器支持则自动尝试 STARTTLS
- `interval_sec`：轮询间隔秒数，**最小 60**，后端对小于 60 的值静默钳制为 60（<=0 或缺失时默认 60）
- `web_url`：点击系统通知后跳转的网页版邮箱地址，允许为空（为空则通知不带 deeplink）
- `password`：**永不在任何 GET 响应中返回**；只有 `has_password` 布尔位
- `status` 为只读运行时状态，POST/PUT 请求体中忽略该字段；`status.last_mail` 可为 `null`

### Event（事件，用于前端「最近事件」面板）

```json
{
  "id": 128,
  "time": "2026-07-22T14:05:03+08:00",
  "account_id": "9f2c1a7e4b6d4f2e",
  "account_name": "QQ 邮箱",
  "kind": "new_mail",
  "detail": "张三 <zhangsan@example.com>：本周例会通知"
}
```

- `kind`：`new_mail`（发现新邮件并已发起通知）、`notify_failed`（通知发送失败）、`check_failed`（巡检失败）、`info`（其他信息，如账号基线建立）

## 端点

| 方法 | 路径 | 说明 | 请求体 | 响应 |
| --- | --- | --- | --- | --- |
| GET | `/api/health` | 健康检查（public，无需登录头） | - | `{"ok":true}` |
| GET | `/api/accounts` | 列出当前用户的全部账号 | - | `Account[]`（空时为 `[]`，不为 null） |
| POST | `/api/accounts` | 新建账号 | Account（无 id/status/created_at/updated_at；含 `password` 明文） | 创建后的 Account |
| PUT | `/api/accounts/{id}` | 更新账号 | Account（`password` 为空字符串表示**不修改**原密码） | 更新后的 Account |
| DELETE | `/api/accounts/{id}` | 删除账号 | - | `{"ok":true}` |
| POST | `/api/accounts/{id}/test` | 测试连接（拨号+登录+列邮箱，不改动巡检状态） | - | `{"ok":true}` 或 `{"ok":false,"error":"..."}`（HTTP 均为 200） |
| POST | `/api/accounts/{id}/check` | 立即触发一次巡检（异步执行） | - | `{"ok":true}` |
| GET | `/api/events?limit=50` | 最近事件，按时间倒序 | - | `Event[]`（空时为 `[]`） |

其他规则：

- 对不属于当前 uid 或不存在的 id 操作，一律返回 404 `{"error":"账号不存在"}`
- 新建账号字段校验：name/protocol/host/port/username/password 必填（port 范围 1-65535，protocol 仅限 imap/pop3），失败返回 400 与中文错误信息

## 巡检与新邮件判定语义（后端内部行为，前端无需实现）

1. 每个 enabled 账号一个独立轮询协程，按 `interval_sec` 触发；同一账号单 flight
2. **首次成功巡检只建立基线，不发通知**（避免历史邮件轰炸）
3. IMAP：记录 `UIDVALIDITY` + 最大 UID；`UIDVALIDITY` 变化则重建基线；新邮件 = UID 大于已记录最大 UID
4. POP3：用 `UIDL` 维护已知邮件集合（上限 5000 条，超出裁剪最旧）；新邮件 = UIDL 不在集合中
5. 发现新邮件：逐封发系统通知（单次巡检最多逐封 3 封，超出部分合并为一条汇总通知）
   - 标题：`【{账号名}】新邮件`
   - 正文：`{发件人}\n{主题}`（汇总通知：`共 {n} 封新邮件，最新：{发件人} - {主题}`）
   - deeplink：账号的 `web_url`（为空则不带）
   - 通知发送到该账号所属 uid 的**所有在线客户端设备**
6. 巡检/通知结果写入事件流（内存环形缓冲，容量 200，无需持久化）
