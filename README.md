# MGate Agent

一个小而安全、可审计、只允许白名单动作的随身 WiFi 远程管理 Agent。

## 当前阶段

`v0.1.0-rc1`：Release 产物契约收口。

当前重点不是增加远程业务能力，而是让 GitHub Release 产物命名稳定、内容完整、可校验、可被外部安装器安全消费。

## 当前能力

- YAML 主配置，默认路径 `/etc/mgate-agent/agent.yaml`。
- credentials 仍为 JSON，默认路径 `/var/lib/mgate-agent/credentials.json`。
- WebSocket 主通道，握手使用 HMAC header。
- HTTPS Pull 兜底通道，Pull request 和 result POST 使用 HMAC header。
- WebSocket 和 Pull 都复用 `commands.Handler`。
- result outbox 持久化补发，默认路径 `/var/lib/mgate-agent/outbox`。
- `check` 严格启动前自检。
- `doctor` 脱敏诊断摘要和轻量网络探测。
- 本地 fake cloud smoke test 覆盖 WS、Pull、outbox 补发。
- systemd unit、安装脚本、卸载脚本和 release tar.gz 目标。

## 安全边界

```text
mgate-cloud
  -> WebSocket / HTTPS Pull
    -> internal/transport
      -> internal/commands.Handler
        -> internal/actions
        -> internal/runner
          -> mgate.sh
```

result 可靠性链路：

```text
commands.Handler
  -> result envelope
    -> outbox
      -> result dispatcher
        -> WebSocket result / HTTPS result POST
```

outbox 只保存 result，不保存 command，不会触发本地命令重放。

## 快速开始

```sh
go test ./...
go vet ./...
go build -o bin/mgate-agent ./cmd/mgate-agent
./bin/mgate-agent version
```

生成默认配置：

```sh
mgate-agent config default > /etc/mgate-agent/agent.yaml
```

## 安装到 Debian 设备

```sh
make release VERSION=v0.1.0-rc1
cd dist
sha256sum -c checksums.txt
```

根据设备架构选择 release 包：

| 设备架构 | Release 包 |
| --- | --- |
| `x86_64` | `mgate-agent-v0.1.0-rc1-linux-amd64.tar.gz` |
| `aarch64` | `mgate-agent-v0.1.0-rc1-linux-arm64.tar.gz` |
| `armv7l` | `mgate-agent-v0.1.0-rc1-linux-armv7.tar.gz` |

在设备上解包后执行：

```sh
tar -xzf mgate-agent-v0.1.0-rc1-linux-arm64.tar.gz
cd mgate-agent-v0.1.0-rc1-linux-arm64
sudo scripts/install.sh
```

安装脚本会创建：

- `/etc/mgate-agent`
- `/var/lib/mgate-agent`
- `/var/lib/mgate-agent/outbox`
- `/var/log/mgate-agent`

如果 `/etc/mgate-agent/agent.yaml` 已存在，安装脚本不会覆盖。安装脚本也不会生成假的 credentials。

## 自检

```sh
mgate-agent check --config /etc/mgate-agent/agent.yaml
```

`check` 是严格启动前检查，会验证配置、credentials、`mgate.sh`、work_dir、audit 目录、outbox 目录、allow_actions 和 transport 配置。有 `[FAIL]` 时返回码为 1。

## Doctor

```sh
mgate-agent doctor --config /etc/mgate-agent/agent.yaml
```

`doctor` 用于部署排错，会输出脱敏配置摘要、启用的 transport、允许 action、outbox pending 数量和短超时网络探测。它不会输出 `device_secret`、signature、psk 或 token。

## systemd

```sh
sudo systemctl daemon-reload
sudo systemctl enable mgate-agent
sudo systemctl start mgate-agent
sudo journalctl -u mgate-agent -f
```

v0.1.0 暂时以 root 运行，因为 `mgate.sh` 后续需要管理 iptables、wlan、AP 等系统能力。systemd hardening 保持保守，后续会根据真实 `mgate.sh` 访问路径逐步收紧。

## Outbox

默认路径：

```text
/var/lib/mgate-agent/outbox
```

默认限制：

- 最多 100 条 result。
- 最多 5MB。

超过限制会优先丢弃最旧 result，并写入 audit。result stdout/stderr 会保留必要输出，但会做最大长度限制和基础敏感行脱敏。

## Release

```sh
make release VERSION=v0.1.0-rc1
```

生成：

```text
dist/mgate-agent-v0.1.0-rc1-linux-amd64.tar.gz
dist/mgate-agent-v0.1.0-rc1-linux-arm64.tar.gz
dist/mgate-agent-v0.1.0-rc1-linux-armv7.tar.gz
dist/checksums.txt
```

校验：

```sh
cd dist
sha256sum -c checksums.txt
```

release 目标依赖 `tar` 和 `sha256sum`，主要面向 Linux CI 环境。

每个 release 包包含二进制、示例配置、systemd unit、安装/卸载脚本、`docs/`、README 和 LICENSE。真机验收步骤见 [docs/device-acceptance.md](docs/device-acceptance.md)，rc1 说明见 [docs/release-notes/v0.1.0-rc1.md](docs/release-notes/v0.1.0-rc1.md)。

CI 会在普通 push / PR 中构建 release 包并上传 `dist/` artifact。tag 推送会触发 `Release Artifacts` workflow，按 tag 名生成同名 release 包和 `checksums.txt`，并上传 artifact。RC 阶段可以手工创建 GitHub Release，再上传这些产物。

### 自动化安装契约

mgate-agent 的 Release 产物保持稳定命名，便于外部安装器下载和校验：

- `mgate-agent-<tag>-linux-amd64.tar.gz`
- `mgate-agent-<tag>-linux-arm64.tar.gz`
- `mgate-agent-<tag>-linux-armv7.tar.gz`
- `checksums.txt`

外部安装器应根据设备架构下载对应包，并使用 `checksums.txt` 校验 SHA256。`credentials.json` 不会被打入 release 包，也不应由自动安装流程伪造。

## 当前不支持

- enroll。
- mgate-cloud 服务端。
- command 持久化或 command 重放。
- `result_ack` 强确认。
- cloud 下发动态命令模板。
- 本地 AP/TProxy/wlan/mihomo 业务逻辑重写。
- MQTT、Redis、SQLite 或第三方队列。
