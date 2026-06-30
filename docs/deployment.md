# 设备部署

本文档面向随身 WiFi Debian 设备部署 `mgate-agent`。推荐优先使用 GitHub Release assets，不建议在资源有限的设备上编译。

## 1. 获取 Release 包

在 GitHub Release 页面选择目标 tag，并按设备架构下载对应包：

| 设备架构 | Release 包后缀 |
| --- | --- |
| `x86_64` | `linux-amd64` |
| `aarch64` | `linux-arm64` |
| `armv7l` | `linux-armv7` |

Release assets 命名规则：

```text
mgate-agent-<tag>-linux-amd64.tar.gz
mgate-agent-<tag>-linux-arm64.tar.gz
mgate-agent-<tag>-linux-armv7.tar.gz
checksums.txt
```

## 2. 校验包

下载 tar.gz 和 `checksums.txt` 后，在同一目录执行：

```sh
sha256sum -c checksums.txt
```

也可以只校验单个架构包：

```sh
grep 'mgate-agent-<tag>-linux-arm64.tar.gz' checksums.txt | sha256sum -c -
```

`<tag>` 替换为 GitHub Release 的实际 tag。

## 3. 上传到设备

示例：

```sh
scp mgate-agent-<tag>-linux-arm64.tar.gz root@DEVICE_IP:/tmp/
```

具体架构按设备选择。

## 4. 解压和安装

在设备上执行：

```sh
cd /tmp
tar -xzf mgate-agent-<tag>-linux-arm64.tar.gz
cd mgate-agent-<tag>-linux-arm64
sh scripts/install.sh
```

安装脚本会创建：

- `/etc/mgate-agent`
- `/var/lib/mgate-agent`
- `/var/lib/mgate-agent/outbox`
- `/var/log/mgate-agent`

如果配置或 credentials 已存在，安装脚本不会覆盖，也不会生成假的 credentials。

## 5. 准备配置

主配置路径：

```text
/etc/mgate-agent/agent.yaml
```

生成模板：

```sh
mgate-agent config default > /etc/mgate-agent/agent.yaml
```

重点编辑：

- `cloud.base_url`
- `cloud.ws_path`
- `cloud.pull_path`
- `cloud.result_path`
- `agent.mgate_path`
- `security.allow_actions`

## 6. 准备 credentials

credentials 仍是 JSON：

```text
/var/lib/mgate-agent/credentials.json
```

Linux 权限必须是：

```sh
chmod 600 /var/lib/mgate-agent/credentials.json
```

不要把 `device_secret` 写入文档、工单、聊天记录或脚本输出。

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

`mgate-agent` Release 为外部安装器提供稳定资产命名和 SHA256 校验文件。外部安装器应下载对应架构包，并在安装前使用 `checksums.txt` 校验。

Release 包不包含真实 credentials，也不会生成假的 credentials。Actions artifact 只用于流水线排错，不是稳定下载源；自动化安装应使用 GitHub Release assets。

## 常见路径

| 项 | 路径 |
| --- | --- |
| 配置 | `/etc/mgate-agent/agent.yaml` |
| 凭证 | `/var/lib/mgate-agent/credentials.json` |
| 工作目录 | `/var/lib/mgate-agent` |
| outbox | `/var/lib/mgate-agent/outbox` |
| audit | `/var/log/mgate-agent/audit.jsonl` |
| systemd unit | `/etc/systemd/system/mgate-agent.service` |
