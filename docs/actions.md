# Actions

> 适配说明：本页记录 agent 代码内已有的远程 action registry。`mgate.sh` 只读状态采集不通过 action registry 暴露，也不会新增远程控制 action。后续如需控制 AP / TProxy / wlan / mihomo，必须另行设计 agent-safe action API。详见 [mgate.sh 契约](mgate-sh-contract.md)。

Action 是 cloud 与本地 `mgate.sh` 之间唯一允许的远程执行边界。所有 action 仍然硬编码在 agent 内，不支持远端动态扩展。

WebSocket 和 Pull 收到 action 后必须交给 `commands.Handler`，由 handler 统一完成 action registry、`allow_actions` 和参数校验。

| Action | 参数 | mgate.sh 映射 | 超时 | 长操作 | 敏感字段 |
| --- | --- | --- | ---: | --- | --- |
| `status.snapshot` | 无 | `mgate.sh status --json` | 10s | 否 | 无 |
| `gateway.status` | 无 | `mgate.sh gateway status --json` | 10s | 否 | 无 |
| `gateway.start` | `country` 必填，`^[A-Z]{2}$` | `mgate.sh gateway start --country <country>` | 60s | 是 | 无 |
| `gateway.stop` | 无 | `mgate.sh gateway stop` | 60s | 是 | 无 |
| `wlan.scan` | 无 | `mgate.sh wlan scan --json` | 30s | 否 | 无 |
| `wlan.switch.safe` | `ssid` 必填，长度 1..32；`psk` 必填，长度 8..64 | `mgate.sh wlan switch-safe --ssid <ssid> --psk <psk> --json` | 180s | 是 | `psk` |

## 参数校验

默认规则：

- 未知 action 拒绝。
- 未知字段拒绝。
- 必填字段缺失拒绝。
- 字段类型不匹配拒绝。
- 字符串长度越界拒绝。
- 明显命令注入形态拒绝。

虽然 runner 不经过 shell，参数校验仍会拒绝 `..`、反引号、命令替换、分号、逻辑连接符和换行等危险形态。这是为了在 `mgate.sh` 后续演进时保留多层防线。

## 超时

每个 action 的超时上限由 action spec 固定。command 未指定 `timeout_sec` 时使用 action 默认值；显式指定 `timeout_sec <= 0` 或超过 action 上限时会被拒绝。

## 敏感字段

`wlan.switch.safe.psk` 不允许进入日志或 audit。audit 中统一写为：

```text
***REDACTED***
```

result outbox 保存的是最终 result envelope，不保存 command args，因此不会持久化 `psk` 参数。
