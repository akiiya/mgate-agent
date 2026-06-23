# v0.1.0-rc1 发布前审计

审计目标：确认 `mgate-agent v0.1.0-rc1` 仍然保持安全远程执行器边界，没有引入远程任意命令执行风险。

## 远程命令执行边界

确认链路：

```text
WebSocket / Pull
  -> commands.Handler
    -> actions.Registry
    -> runner
      -> mgate.sh argv
```

审计结果：

- transport 生产代码不直接导入 runner。
- transport 生产代码不直接导入 actions。
- cloud 只能下发 action，不支持任意命令行。
- action 只能来自硬编码 registry。
- `security.allow_actions` 仍作为本机二次收窄。
- runner 只使用 `exec.CommandContext(ctx, mgatePath, argv...)`。
- 安全门禁测试覆盖 shell 中转执行风险。

## 敏感信息边界

审计结果：

- `device_secret` 只从 credentials JSON 读取。
- HMAC signature 放在 header，不放 URL。
- doctor 输出 `credentials.device_secret: ***REDACTED***`。
- audit 不记录 stdout/stderr 全量内容。
- result stdout/stderr 有最大长度限制。
- result stdout/stderr 对明显敏感行做基础脱敏。
- outbox 保存前再次脱敏 result payload。

## Outbox 边界

审计结果：

- outbox 只接受 `type=result` envelope。
- outbox 不保存 command。
- outbox 不会调用 runner 或 action registry。
- result 发送失败只更新 attempts、last_error 和 next_retry_at。
- result 发送失败不会触发 command 重放。
- outbox 默认最多 100 条、5MB。
- 损坏 JSON 文件会被重命名为 `.bad`，不会导致 agent 崩溃。

## Transport 边界

审计结果：

- WebSocket 握手使用 HMAC header。
- Pull request 使用 HMAC header。
- result POST 使用 HMAC header。
- secret/signature 不进入 URL。
- WebSocket 和 Pull 共用 `commands.Handler` dedupe。
- WebSocket 健康时 Pull 暂停高频轮询。
- WebSocket 断开时 Pull 启用。
- `ws_enabled=false && pull_enabled=true` 时 Pull 可作为主通道运行。

## 发布结论

代码结构仍符合 v0.1.0 的安全设计：transport 只负责收发，执行入口仍是 `commands.Handler`，本地执行仍由 action registry 和 runner 安全封装控制。
