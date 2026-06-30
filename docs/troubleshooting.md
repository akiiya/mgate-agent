# 排障指南

优先运行：

```sh
mgate-agent doctor --config /etc/mgate-agent/agent.yaml
```

`doctor` 输出可以直接复制给开发者排查；其中不会包含 `device_secret`。

## 配置文件不存在

现象：

```text
[FAIL] 配置文件不可读取
```

处理：

```sh
mgate-agent config default > /etc/mgate-agent/agent.yaml
```

然后编辑配置。

## YAML 解析失败

检查缩进、冒号和字段名。主配置拒绝 unknown field，拼写错误会直接失败。

## credentials 权限不是 0600

Linux 设备上执行：

```sh
chmod 600 /var/lib/mgate-agent/credentials.json
```

不要把 credentials 放到共享目录，也不要把 secret 发到日志或工单。

## mgate.sh 不存在或不可执行

确认：

```sh
ls -l /usr/local/bin/mgate.sh
chmod +x /usr/local/bin/mgate.sh
```

`agent.mgate_path` 必须是绝对路径。

## WebSocket 连接失败

检查：

- `cloud.base_url`
- `cloud.ws_path`
- DNS 和网络出口
- Cloudflare / 反向代理是否允许 WebSocket
- 服务端是否能验证 HMAC header

WebSocket 不通且 `pull_enabled=true` 时，agent 会启用 Pull 兜底。

## Pull 失败

检查：

- `cloud.pull_path`
- `cloud.result_path`
- HTTP 状态码
- 服务端 HMAC 验签
- 设备时间是否偏差过大

## outbox 堆积

`doctor` 会显示 `outbox_pending`。

常见原因：

- WebSocket 长期不可用。
- result POST 返回非 2xx。
- cloud 服务端无法验签。
- 网络断开。

outbox 默认最多 100 条、5MB，超过后会丢弃最旧 result。

## action 被拒绝

检查：

- action 是否存在于代码内 registry。
- action 是否在 `security.allow_actions` 中。
- 参数是否符合校验规则。

## command_id 重复

WebSocket 和 Pull 共用 dedupe。同一个 `command_id` 重复到达时，后续 command 会被 rejected，避免重复执行本地动作。

## systemd 启动失败

查看：

```sh
systemctl status mgate-agent
journalctl -u mgate-agent -n 100
```

常见原因：

- 配置路径不存在。
- credentials 未创建。
- `mgate.sh` 不可执行。
- `/var/lib/mgate-agent` 或 `/var/log/mgate-agent` 权限异常。
