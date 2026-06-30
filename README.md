# MGate Agent

一个小而安全、可审计、只允许白名单动作的随身 WiFi 远程管理 Agent。

`mgate-agent` 运行在刷 Debian 的随身 WiFi / 小型 Linux 设备上，负责连接云端、校验消息、调度本地白名单 action、回传结果，并采集本机 `mgate.sh` 的只读状态。

它不实现 AP、TProxy、wlan、mihomo 等本地业务逻辑；这些能力由 `mgate.sh` 提供。

## ✨ 能力概览

- YAML 主配置：`/etc/mgate-agent/agent.yaml`
- JSON credentials：`/var/lib/mgate-agent/credentials.json`
- WebSocket 主通道，握手使用 HMAC header
- HTTPS Pull 兜底通道，Pull request / result POST 使用 HMAC header
- 统一 `commands.Handler` 安全门禁
- result outbox 持久化补发：`/var/lib/mgate-agent/outbox`
- `mgate.sh` 只读状态采集：`capabilities-json` / `agent-snapshot`
- `check` 启动前自检
- `doctor` 脱敏诊断摘要
- systemd unit、安装脚本、卸载脚本和 Release 打包目标

## 🔐 安全边界

```text
mgate-cloud
  -> WebSocket / HTTPS Pull
    -> internal/transport
      -> internal/commands.Handler
        -> internal/actions
        -> internal/runner
          -> mgate.sh argv
```

核心原则：

- 不接受任意远程 shell。
- 不使用 `sh -c` / `bash -c`。
- cloud 只能下发 action，不能下发命令模板。
- action 必须存在于代码内 registry，并被本机 `allow_actions` 允许。
- outbox 只保存 result，不保存 command，也不会触发命令重放。
- secret、psk、token、signature 不写入日志、audit 或诊断输出。

## 🚀 快速开始

```sh
go test ./...
go vet ./...
go build -o bin/mgate-agent ./cmd/mgate-agent
```

生成默认配置：

```sh
mgate-agent config default > /etc/mgate-agent/agent.yaml
```

检查设备配置：

```sh
mgate-agent check --config /etc/mgate-agent/agent.yaml
mgate-agent doctor --config /etc/mgate-agent/agent.yaml
```

启动：

```sh
mgate-agent run --config /etc/mgate-agent/agent.yaml
```

## ⚙️ 配置与凭证

主配置使用 YAML，适合人工编辑：

```text
/etc/mgate-agent/agent.yaml
```

设备凭证仍使用 JSON，并且在 Linux 下权限必须是 `0600`：

```text
/var/lib/mgate-agent/credentials.json
```

安装脚本不会覆盖已有配置，也不会生成假的 credentials。

## 📡 mgate.sh 只读状态

agent 只读取本机 `mgate.sh` 状态，不新增远程控制 action：

- 启动时调用 `mgate capabilities-json`
- heartbeat / Pull request 中调用 `mgate agent-snapshot`
- 摘要包含 wifi、ap、gateway、tproxy、mihomo、subscription、web
- 采集失败只上报稳定 `error_code`，不影响 WebSocket、Pull 或 outbox

不支持通过 cloud 触发：

- `wifi-connect`
- `gateway-start`
- `tproxy-start`
- `ap-start`

远程控制必须另行设计 action API、安全事务、操作锁和回滚机制。

## 🧰 systemd

```sh
sudo systemctl daemon-reload
sudo systemctl enable mgate-agent
sudo systemctl start mgate-agent
sudo journalctl -u mgate-agent -f
```

systemd hardening 保持保守，避免误伤 `mgate.sh` 后续对网络栈的合法管理需求。

## 📦 Release 产物

Release 版本来自 GitHub Release tag。仓库不维护版本文件。

本地手工构建时显式传入 tag：

```sh
make release VERSION=<tag>
make verify-release VERSION=<tag>
```

产物命名：

```text
mgate-agent-<tag>-linux-amd64.tar.gz
mgate-agent-<tag>-linux-arm64.tar.gz
mgate-agent-<tag>-linux-armv7.tar.gz
checksums.txt
```

校验：

```sh
cd dist
sha256sum -c checksums.txt
```

外部安装器应下载对应架构包，并使用 `checksums.txt` 校验 SHA256。Release notes 直接维护在 GitHub Release 页面。

## 📚 文档

- [架构](docs/architecture.md)
- [安全设计](docs/security.md)
- [协议](docs/protocol.md)
- [Action 契约](docs/actions.md)
- [mgate.sh 契约](docs/mgate-sh-contract.md)
- [部署指南](docs/deployment.md)
- [真机验收](docs/device-acceptance.md)
- [排错指南](docs/troubleshooting.md)
- [发布流程](docs/branching-and-release.md)
- [发布检查清单](docs/release-checklist.md)
- [发布前审计](docs/pre-release-audit.md)

## 🚫 不做什么

- 不实现 enroll。
- 不实现 mgate-cloud 服务端。
- 不实现 `result_ack` 强确认。
- 不持久化 command。
- 不重放 command。
- 不新增远程控制 action。
- 不重写 AP / TProxy / wlan / mihomo 业务逻辑。
- 不引入 MQTT、Redis、SQLite 或第三方队列。
