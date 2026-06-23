# 安全设计

`mgate-agent` 的安全目标不是“能执行命令”，而是“只能执行被允许的本地动作”。

## HMAC Header

WebSocket 握手、HTTPS Pull request 和 result POST 都使用同一组 HMAC header：

```text
X-MGate-Device-ID
X-MGate-Tenant-ID
X-MGate-Timestamp
X-MGate-Nonce
X-MGate-Signature
X-MGate-Agent-Version
```

签名内容：

```text
METHOD
PATH
TIMESTAMP
NONCE
SHA256(BODY)
```

签名使用 `HMAC-SHA256`，结果使用 base64。`device_secret` 来自 credentials JSON。secret 和 signature 不放入 URL query，避免被代理访问日志、浏览器历史或错误日志记录。

## Credentials

主配置使用 YAML，适合人工编辑。设备凭证仍然使用 JSON：

```text
/var/lib/mgate-agent/credentials.json
```

原因是 credentials 包含 `device_secret`，应保持机器友好、稳定、最小化。Linux 下权限必须是 `0600`。

## Transport 不执行命令

WebSocket 和 Pull 都只是 transport。它们只能：

- 建立连接或发起 HTTP 请求。
- 发送 hello、heartbeat、ack、result。
- 接收 envelope。
- 校验 envelope 基础结构。
- 将 command 放入有界队列。

它们不能直接调用 runner，不能解析或拼接命令行。所有 command 必须进入 `commands.Handler`。

## 双层白名单

Agent 使用双层白名单：

- 代码内 action registry：定义系统知道哪些 action。
- 本地 YAML 配置 `security.allow_actions`：定义这台设备允许哪些 action。

cloud 即使能下发某个 action，本机也可以通过配置进一步收窄授权面。

## command_id 去重

WebSocket 和 Pull 共用同一个 `commands.Handler`，因此共享 command dedupe。即使两个 transport 收到相同 `command_id`，也只会有一个进入执行路径，另一个会被 rejected。

## Outbox 边界

Phase 4 的 outbox 只保存 result envelope，不保存 command。

这条边界很重要：result 发送失败只能触发 result 补发，不能触发本地命令重放。否则网络抖动可能导致重复改写设备状态。

默认路径：

```text
<agent.work_dir>/outbox
```

默认限制：

- 最多 100 条 result。
- 最多 5MB。

超过限制时优先丢弃最旧 result，并写入 audit。v0.1.0 的 outbox 是轻量可靠性补强，不是无限队列。

## Outbox 文件安全

- 文件写入使用同目录临时文件 + fsync + rename，避免半截 JSON 被当成有效 record。
- 启动时会忽略或清理 `.tmp` 文件。
- 损坏 JSON 会被重命名为 `.bad`，并写入 `outbox_record_corrupted` audit。
- outbox 不应保存 `device_secret`、`psk`、`token` 等敏感字段。
- audit 不记录 stdout/stderr 全量内容。

## Result 输出脱敏

cloud 需要看到必要 stdout/stderr 来排障，因此 agent 不会盲目删除 result 输出。

v0.1.0 做两层基础防护：

- runner 已有最大输出长度限制，避免输出无限增长。
- stdout/stderr 中包含 `psk`、`password`、`passwd`、`token`、`secret`、`key`、`device_secret`、`signature` 等关键词的整行会被替换为 `***REDACTED***`。

这不是完整 DLP 系统，只是 RC 阶段的基础泄露防护。`mgate.sh` 仍应避免主动输出 WiFi 密码、token 或 secret。

## 发送成功语义

Phase 4 暂不实现 `result_ack` 强确认。

- WebSocket：result envelope 写入 WebSocket 成功，即认为发送成功。
- HTTPS Pull：result POST 返回 HTTP 2xx，即认为发送成功。

如果发送失败，record 会保留在 outbox，更新 attempts、last_error 和 next_retry_at，等待后续补发。

## 禁止任意 shell

runner 只使用：

```go
exec.CommandContext(ctx, mgatePath, argv...)
```

其中 `mgatePath` 来自本地 YAML 配置，`argv` 来自硬编码 action spec 和已校验参数。项目包含安全门禁测试，用于防止引入 shell 中转执行。

## 日志与审计

日志不打印 stdout/stderr 全量内容，不打印 secret、signature、psk 或 token。

Audit 使用 JSONL，覆盖：

- `command_received`
- `command_rejected`
- `command_started`
- `command_finished`
- `outbox_record_saved`
- `outbox_record_send_started`
- `outbox_record_send_succeeded`
- `outbox_record_send_failed`
- `outbox_record_deleted`
- `outbox_record_dropped`
- `outbox_record_corrupted`

敏感字段会递归脱敏为：

```text
***REDACTED***
```

## Doctor 输出

`mgate-agent doctor` 只输出安全摘要：配置路径、启用 transport、允许 action、outbox pending、device_id 和 tenant_id。它不会输出 `device_secret`、signature、psk 或 token。
