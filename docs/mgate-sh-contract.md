# mgate.sh 契约

本文档描述 `mgate-agent` 期望的本地 `mgate.sh` 命令契约。agent 不在内部补业务逻辑；如果真实 `mgate.sh` 暂未完全对齐，应优先调整 `mgate.sh` 或在后续阶段明确迁移方案。

## 期望命令

```text
mgate.sh status --json
mgate.sh gateway status --json
mgate.sh gateway start --country <country>
mgate.sh gateway stop
mgate.sh wlan scan --json
mgate.sh wlan switch-safe --ssid <ssid> --psk <psk> --json
```

## 输出

Agent 会分别捕获 stdout 和 stderr：

- stdout：建议输出机器可读 JSON。
- stderr：建议输出诊断信息。

Agent 不解析业务 JSON，只负责回传、截断和记录执行状态。因此 `mgate.sh` 应保证自身输出稳定。

## 退出码

- `0`：动作成功。
- 非 `0`：动作失败。

Agent 会把非零退出码映射为 `failed`，超时映射为 `timed_out`。

## 超时

各 action 默认超时由 agent 固定：

- `status.snapshot`：10 秒。
- `gateway.status`：10 秒。
- `gateway.start`：60 秒。
- `gateway.stop`：60 秒。
- `wlan.scan`：30 秒。
- `wlan.switch.safe`：180 秒。

## 敏感参数

`wlan.switch.safe` 的 `psk` 是敏感字段。`mgate.sh` 不应把该值写入 stdout、stderr 或系统日志。agent 的 audit 会脱敏，但 runner result 和 outbox 需要回传最终 stdout/stderr，因此本地脚本也必须避免泄露。
