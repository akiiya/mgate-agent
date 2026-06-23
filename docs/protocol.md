# 协议

Phase 4 复用统一 envelope。WebSocket 和 HTTPS Pull 只是不同 transport，command 与 result 的语义保持一致。

## Envelope

```json
{
  "version": "1",
  "type": "command",
  "message_id": "msg_xxx",
  "correlation_id": "cmd_xxx",
  "device_id": "dev_xxx",
  "timestamp": "2026-06-23T00:00:00Z",
  "payload": {}
}
```

当前类型：

```text
hello
hello_ack
heartbeat
command
ack
result
error
```

## hello

连接建立后 agent 立即发送 `hello`，包含版本、设备标识、允许 action 和基础能力。hello 不包含 `device_secret`、WiFi 密码或 token。

能力中 `outbox=true` 表示 agent 支持 result 持久化补发。

## hello_ack

```json
{
  "accepted": true,
  "server_time": "2026-06-23T00:00:00Z",
  "message": "ok"
}
```

如果 `accepted=false` 或等待超时，agent 会断开并按 backoff 重连。

## heartbeat

```json
{
  "agent_version": "v0.1.0",
  "device_id": "dev_example",
  "uptime_sec": 123,
  "active_jobs": 0,
  "last_command_id": "cmd_xxx",
  "last_command_state": "succeeded",
  "outbox_pending": 3
}
```

`outbox_pending` 是轻量状态，用于提示还有多少 result 等待补发。

## command

cloud 下发 command envelope。transport 只校验 envelope version 和 device_id，然后把 payload 交给 `commands.Handler`。

## ack

agent 成功把 command 放入本地有界队列后发送 ack：

```json
{
  "command_id": "cmd_xxx",
  "action": "status.snapshot",
  "state": "queued",
  "accepted": true,
  "error_code": ""
}
```

ack 只表示“已收到并排队”，不代表命令成功。最终状态以 result 为准。

## result

```json
{
  "command_id": "cmd_xxx",
  "action": "status.snapshot",
  "state": "succeeded",
  "exit_code": 0,
  "stdout": "...",
  "stderr": "",
  "output_truncated": false,
  "started_at": "2026-06-23T00:00:01Z",
  "ended_at": "2026-06-23T00:00:05Z",
  "duration_ms": 4000,
  "error_code": ""
}
```

final result 会先写入 outbox，再通过 WebSocket 或 HTTPS result POST 发送。

stdout/stderr 会保留必要诊断信息，但进入 result 前会做最大长度限制和基础敏感行脱敏。

## error

基础 envelope 错误使用 `error` envelope。device_id 不匹配、队列满、action 拒绝等 command 级问题使用 rejected result。

## Pull Request

Pull 使用：

```text
POST /api/agent/v1/pull
```

请求 body：

```json
{
  "agent_version": "v0.1.0",
  "device_id": "dev_example",
  "tenant_id": "tenant_example",
  "device_name": "ufi-001",
  "last_command_id": "cmd_xxx",
  "last_command_state": "succeeded",
  "active_jobs": 0,
  "transport": "pull"
}
```

body 参与 HMAC 签名，不包含 secret、WiFi 密码或 stdout/stderr。

## Pull Response

```json
{
  "server_time": "2026-06-23T00:00:00Z",
  "commands": []
}
```

`commands` 是 command envelope 数组，可以为空。Phase 4 单次最多处理 16 条。

## Result POST

Pull 模式和 outbox 补发都使用：

```text
POST /api/agent/v1/result
```

body 复用 result envelope，并参与 HMAC 签名。HTTP 2xx 表示发送成功；非 2xx 会保留 outbox record 等待重试。

## Outbox Record

outbox 保存 result envelope，不保存 command：

```json
{
  "record_id": "cmd_xxx",
  "command_id": "cmd_xxx",
  "created_at": "2026-06-23T00:00:05Z",
  "updated_at": "2026-06-23T00:00:05Z",
  "attempts": 0,
  "last_error": "",
  "next_retry_at": "2026-06-23T00:00:05Z",
  "envelope": {
    "version": "1",
    "type": "result",
    "message_id": "msg_result_xxx",
    "correlation_id": "cmd_xxx",
    "device_id": "dev_example",
    "timestamp": "2026-06-23T00:00:05Z",
    "payload": {}
  }
}
```

发送失败只更新 attempts、last_error 和 next_retry_at，不会重新执行 command。

## HTTP HMAC

Pull 和 result POST 的 canonical string：

```text
METHOD
PATH
TIMESTAMP
NONCE
SHA256(BODY)
```

签名放在 `X-MGate-Signature` header，secret 和 signature 不进入 URL。

## result_ack

Phase 4 不要求 server 返回 `result_ack`。后续如果需要更强交付语义，可在协议中扩展 result ack。
