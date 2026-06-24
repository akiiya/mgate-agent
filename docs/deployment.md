# 设备部署

本文档面向随身 WiFi Debian 设备部署 `mgate-agent v0.1.0-rc1`。

## 1. 生成 Release 包

推荐在电脑或 CI 上构建，不建议在资源有限的设备上编译。

```sh
make release
```

生成产物：

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

## 2. 选择架构

| 设备架构 | Release 包 |
| --- | --- |
| `x86_64` | `linux-amd64` |
| `aarch64` | `linux-arm64` |
| `armv7l` | `linux-armv7` |

## 3. 上传到设备

示例：

```sh
scp dist/mgate-agent-v0.1.0-rc1-linux-arm64.tar.gz root@DEVICE_IP:/tmp/
```

## 4. 解压和安装

在设备上执行：

```sh
cd /tmp
tar -xzf mgate-agent-v0.1.0-rc1-linux-arm64.tar.gz
cd mgate-agent-v0.1.0-rc1-linux-arm64
sh scripts/install.sh
```

安装脚本会创建：

- `/etc/mgate-agent`
- `/var/lib/mgate-agent`
- `/var/lib/mgate-agent/outbox`
- `/var/log/mgate-agent`

如果配置已存在，不会覆盖。

## 5. 准备配置

默认配置：

```text
/etc/mgate-agent/agent.yaml
```

也可以生成新模板：

```sh
mgate-agent config default > /etc/mgate-agent/agent.yaml
```

编辑 `cloud.base_url`、`agent.mgate_path` 和 `security.allow_actions`。

## 6. 准备 credentials

credentials 仍是 JSON：

```text
/var/lib/mgate-agent/credentials.json
```

Linux 权限必须是：

```sh
chmod 600 /var/lib/mgate-agent/credentials.json
```

安装脚本不会生成假的 credentials，也不会输出 secret。

## 7. 运行自检

```sh
mgate-agent check --config /etc/mgate-agent/agent.yaml
```

如需排障摘要：

```sh
mgate-agent doctor --config /etc/mgate-agent/agent.yaml
```

## 8. 启动 systemd

```sh
sudo systemctl daemon-reload
sudo systemctl enable mgate-agent
sudo systemctl start mgate-agent
```

查看日志：

```sh
journalctl -u mgate-agent -f
```

## 自动化安装契约

`mgate-agent` Release 为外部安装器提供稳定资产命名：

```text
mgate-agent-<tag>-linux-amd64.tar.gz
mgate-agent-<tag>-linux-arm64.tar.gz
mgate-agent-<tag>-linux-armv7.tar.gz
checksums.txt
```

外部安装器应下载对应架构包，并在安装前使用 `checksums.txt` 校验 SHA256。Release 包不包含真实 credentials，也不会生成假的 credentials。

`VERSION` 文件是默认版本来源。代码 merge 到 `main` 后，`Main Release` workflow 会读取 `VERSION`，先完成测试、构建、打包和 checksum 校验，再自动创建 tag、GitHub Release 并上传：

- 三个 Linux tar.gz。
- `checksums.txt`。

`-rc`、`-beta`、`-alpha` 版本会自动标记为 pre-release。如果同名 tag 或 Release 已存在，workflow 不会覆盖旧资产；需要重发时应更新 `VERSION`，例如 `v0.1.0-rc2`。

Actions artifact 只用于流水线排错，不是稳定下载源。外部安装器应使用 GitHub Release assets。

## 常见路径

| 项 | 路径 |
| --- | --- |
| 配置 | `/etc/mgate-agent/agent.yaml` |
| 凭证 | `/var/lib/mgate-agent/credentials.json` |
| 工作目录 | `/var/lib/mgate-agent` |
| outbox | `/var/lib/mgate-agent/outbox` |
| audit | `/var/log/mgate-agent/audit.jsonl` |
| systemd unit | `/etc/systemd/system/mgate-agent.service` |
