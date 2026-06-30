# 真机部署验收

本文档用于在真实随身 WiFi Debian 设备上验收 `mgate-agent`。第一次验证建议连接测试 cloud 或 fake cloud，不建议直接连接生产 cloud。

## 1. 前置条件

- 设备运行 Debian 或兼容环境。
- 有 root 权限。
- 已存在可执行的 `mgate.sh`。
- 已准备 `agent.yaml`。
- 已准备 `credentials.json`。
- 测试 cloud 或 fake cloud 可用。
- 已选择正确 CPU 架构：`linux-arm64` 或 `linux-armv7`。

## 2. 上传 Release 包

示例：

```sh
scp mgate-agent-<tag>-linux-armv7.tar.gz root@DEVICE_IP:/tmp/
scp checksums.txt root@DEVICE_IP:/tmp/
```

具体架构按设备选择。

在设备上校验：

```sh
cd /tmp
grep 'mgate-agent-<tag>-linux-armv7.tar.gz' checksums.txt | sha256sum -c -
```

## 3. 解压和安装

```sh
cd /tmp
tar -xzf mgate-agent-<tag>-linux-armv7.tar.gz
cd mgate-agent-<tag>-linux-armv7
sh scripts/install.sh
```

安装脚本不会覆盖已有 `/etc/mgate-agent/agent.yaml`，也不会生成假的 credentials。

## 4. 准备配置

```sh
mkdir -p /etc/mgate-agent
mgate-agent config default > /etc/mgate-agent/agent.yaml
vi /etc/mgate-agent/agent.yaml
```

重点确认：

- `cloud.base_url`
- `cloud.ws_path`
- `cloud.pull_path`
- `cloud.result_path`
- `agent.mgate_path`
- `security.allow_actions`

## 5. 准备 credentials

```sh
mkdir -p /var/lib/mgate-agent
vi /var/lib/mgate-agent/credentials.json
chmod 600 /var/lib/mgate-agent/credentials.json
```

不要在文档、工单或聊天记录中写真实 `device_secret`。

## 6. 执行 check

```sh
mgate-agent check --config /etc/mgate-agent/agent.yaml
```

期望结果：

```text
结果：通过
```

有 `[FAIL]` 时不要启动服务，先按输出修复。

## 7. 执行 doctor

```sh
mgate-agent doctor --config /etc/mgate-agent/agent.yaml
```

确认：

- 输出包含 transport 状态。
- 输出包含 outbox pending 数量。
- 不输出 `device_secret`、psk、token 或 signature。

## 8. 启动服务

```sh
systemctl enable mgate-agent
systemctl start mgate-agent
systemctl status mgate-agent
```

## 9. 查看日志

```sh
journalctl -u mgate-agent -f
```

日志中不应出现 secret、signature、psk 或 token。

## 10. 验收 WebSocket

1. agent 能连接 cloud。
2. cloud 能收到 `hello`。
3. cloud 能收到 heartbeat。
4. cloud 下发只读测试 command。
5. agent 回传 result。
6. 本地 fake mgate 或真实 `mgate.sh` 只执行一次。

## 11. 验收 Pull

1. 停止或阻断 WebSocket。
2. agent 自动进入 Pull。
3. cloud 通过 Pull response 下发 command。
4. agent 通过 result POST 回传。
5. command 不重复执行。

## 12. 验收 mgate.sh 只读状态

1. 启动时确认 agent 调用 `mgate capabilities-json` 成功。
2. heartbeat 或 Pull request 中能看到 `mgate.available=true`。
3. 摘要包含 wifi、ap、gateway、tproxy、mihomo、subscription、web。
4. 停止或移走 `mgate.sh` 后，agent 只上报稳定 `error_code`，主通道不退出。
5. cloud 不能通过 agent 触发 `wifi-connect`、`gateway-start`、`tproxy-start` 或 `ap-start`。

## 13. 验收 outbox

1. 模拟 result POST 失败或 WebSocket 断开。
2. result 进入 `/var/lib/mgate-agent/outbox`。
3. 恢复连接。
4. result 自动补发。
5. 发送成功后 outbox record 删除。
6. 本地 command 没有重新执行。

## 14. 回滚

停止 agent：

```sh
systemctl stop mgate-agent
systemctl disable mgate-agent
```

卸载服务和二进制：

```sh
sh scripts/uninstall.sh
```

`uninstall.sh` 默认不删除配置、credentials、日志和 outbox。如需删除数据，请确认已备份后手动处理。
