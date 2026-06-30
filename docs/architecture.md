# 架构

`mgate-agent` 是运行在随身 WiFi Debian 设备上的轻量 Go 常驻服务。它不实现 AP、TProxy、wlan、mihomo 等本地业务逻辑，只负责安全远程通道、白名单 action 映射、调用本地 `mgate.sh`、回传 result、采集只读状态和写审计日志。

## 模块划分

```text
cmd/mgate-agent      CLI 入口
internal/app         run/check/version 编排
internal/config      YAML 主配置加载、默认模板和校验
internal/identity    JSON 设备凭证读取和权限检查
internal/protocol    envelope、hello、heartbeat、command、ack、result 结构
internal/mgate       本机 mgate.sh 只读状态采集，调用 capabilities-json / agent-snapshot
internal/transport   WebSocket、HTTPS Pull、Manager、result dispatcher
internal/outbox      result envelope 持久化、读取、删除和容量控制
internal/integration 本地 fake cloud smoke test
internal/commands    command lifecycle 唯一处理入口
internal/auth        HMAC-SHA256 签名与验签基础
internal/actions     本地硬编码 action registry 和参数校验
internal/runner      安全调用 mgate.sh，捕获输出和超时
internal/audit       JSONL 审计日志和敏感字段脱敏
internal/logx        标准库 slog 初始化
```

`internal/app` 还负责 `check` 和 `doctor` 诊断编排。诊断逻辑不执行远程 command，只检查本地部署状态和输出脱敏摘要。

## 主链路

```text
mgate-cloud
  -> WebSocket 主通道
  -> HTTPS Pull 兜底通道
    -> internal/transport
      -> internal/commands.Handler
        -> internal/actions
        -> internal/runner
          -> mgate.sh argv
```

`internal/transport` 只负责连接、收发 envelope、排队和投递 result。它不判断 action 业务语义，不直接调用 runner。

## 只读状态采集链路

agent 会在启动时调用 `mgate capabilities-json`，并在 heartbeat / Pull request 状态中调用 `mgate agent-snapshot` 生成轻量摘要：

```text
internal/transport heartbeat / pull request
  -> internal/mgate
    -> exec.CommandContext(ctx, mgate_path, "agent-snapshot")
      -> JSON schema_version 校验
      -> 脱敏摘要
```

这条链路只读，不进入 action registry，也不允许 cloud 远程触发 `wifi-connect`、`gateway-start`、`tproxy-start`、`ap-start` 等控制命令。mgate 采集失败只会上报 `mgate.available=false` 和稳定 `error_code`，不会导致 WebSocket、Pull 或 outbox 退出。

## Result 可靠性链路

```text
commands.Handler
  -> result envelope
    -> internal/outbox
      -> result dispatcher
        -> WebSocket result
        -> HTTPS result POST
```

outbox 是“回执箱”，不是任务队列。它只保存已经产生的 result，不保存 command，也不会因为发送失败而重新执行本地命令。

## Transport Manager

`Manager` 集中协调 WebSocket、Pull 和 result dispatcher：

- `ws_enabled=true` 时启动 WebSocket。
- `pull_enabled=true` 时启动 Pull loop。
- WebSocket 健康时暂停 Pull 高频轮询。
- WebSocket 不健康时 Pull 按 `pull_interval_sec` 轮询。
- `ws_enabled=false && pull_enabled=true` 时 Pull 作为主通道运行。
- dispatcher 周期性扫描 outbox pending result，并选择可用 transport 补发。

集中协调可以避免两个 transport 各自实现状态判断，也确保它们共享同一个 `commands.Handler`、同一个 dedupe 和同一个 result outbox。

## WebSocket 生命周期

1. 根据 `cloud.base_url + cloud.ws_path` 构造 `ws/wss` URL。
2. 握手时携带 HMAC header。
3. 连接成功后发送 `hello`。
4. 收到 `hello_ack` 且 `accepted=true` 后进入健康状态。
5. 定时发送 heartbeat，包含轻量 outbox pending 计数和 mgate 只读摘要。
6. read loop 接收 command 并放入有界队列。
7. worker 调用 `commands.Handler`。
8. result envelope 先写入 outbox，再由 dispatcher 发送。
9. 连接断开后使用指数退避和 jitter 重连。

## Pull 生命周期

1. WebSocket 不健康或禁用时，Pull 发起 `POST /pull`。
2. Pull request 使用 HMAC header，body 参与签名。
3. Pull request 可携带 mgate 只读摘要。
4. response 中的 command envelope 逐条交给 `commands.Handler`。
5. result envelope 先写入 outbox。
6. dispatcher 可通过 `POST /result` 补发 pending result。

## Command Handler

`internal/commands` 是所有 command 的唯一处理入口：

```text
received
  -> command_id 校验和去重
  -> action registry
  -> config allow_actions
  -> args validation
  -> runner
  -> finished
```

这样 WebSocket 和 Pull 都只是 transport，不会各自长出业务逻辑。

## 诊断链路

```text
mgate-agent check
  -> 配置 / credentials / mgate_path / 目录 / action / transport 严格检查

mgate-agent doctor
  -> check
  -> 脱敏配置摘要
  -> outbox pending
  -> 短超时 cloud base_url 网络探测
```

`doctor` 的目标是部署排错，不会输出 secret、signature、psk 或 token。

## 当前能力边界

已实现：

- WebSocket 主通道。
- HTTPS Pull 兜底通道。
- HMAC header 鉴权基础。
- hello / hello_ack / heartbeat。
- command ack 和 result。
- 有界 command queue 和 worker。
- result outbox 持久化补发。
- outbox 启动加载、atomic write、容量限制和损坏文件隔离。
- mgate.sh 只读状态采集。
- check / doctor。
- systemd、install/uninstall 脚本和 release tar.gz target。
- fake cloud smoke test。

未实现：

- enroll。
- mgate-cloud 服务端。
- command 持久化或 command 重放。
- `result_ack` 强确认。
- 远程控制 AP / TProxy / wlan / mihomo 的 action API。
