# mgate.sh 能力与 agent 适配契约

本文档记录当前 `mgate.sh` 能力边界，作为 `mgate-agent` 后续适配和 cloud 展示的项目记忆。

当前结论：`mgate.sh` 已准备好进入 `mgate-agent` 第一阶段状态采集适配。agent 应优先接入：

```text
mgate capabilities-json
mgate agent-snapshot
```

不要先做远程控制。先让 cloud 稳定“看见”设备，再讨论 cloud 如何“控制”设备。

## agent 当前适配范围

`mgate-agent` 当前只接入两个只读命令：

- `mgate capabilities-json`：启动时低频读取能力契约。
- `mgate agent-snapshot`：heartbeat / Pull request 中读取轻量状态摘要。

当前没有新增任何远程控制 action，也不会通过 cloud 触发 `wifi-connect`、`gateway-start`、`tproxy-start`、`ap-start` 等控制类命令。后续远程控制必须另行设计 action API、安全事务和回滚机制。

## 1. mgate.sh 当前定位

`mgate.sh` 已从“本地 mihomo 代理管理脚本”发展为“随身 WiFi 本地网关管理脚本”。

它在刷 Debian 的随身 WiFi / 小型 Linux 设备上提供：

- Mihomo 本地代理管理。
- Clash/Mihomo YAML 订阅管理。
- AP 热点管理。
- 上级 WiFi 管理。
- 普通 NAT fallback 网关。
- TProxy 透明代理。
- Web 管理后台。
- TUI 菜单。
- JSON 状态接口。
- health / doctor / debug 诊断。
- preflight / CRLF 防护。

`mgate-agent` 应把 `mgate.sh` 视为本机能力提供方。当前适配阶段只负责采集状态并上报 cloud，不应直接调用危险操作。

## 2. 当前稳定状态

已完成真机验证：

- AP 可用，手机可连接 `mgate` 热点。
- 普通 NAT fallback 可用，手机可通过 AP 上网。
- TProxy 透明代理可用，手机流量可进入 mihomo `tproxy-port`。
- `tproxy-stop` 可清理 TProxy 规则并回到 NAT fallback。
- JSON 状态接口真机验证通过。
- `agent-snapshot` / `capabilities-json` 真机验证通过。
- 上级 WiFi 只读命令和 TUI 入口已验证。

等待 agent + cloud 状态上报后再做闭环验证：

- WiFi 危险切换。
- fallback / rollback / watchdog。
- rescue AP。
- 错误密码 profile 回滚。
- WiFi 切换后的 AP / NAT / TProxy 联动恢复。

## 3. 默认值

| 项目 | 默认值 |
| --- | --- |
| Web 端口 | `31888` |
| mixed-port | `31800` |
| tproxy-port | `31802` |
| AP interface | `ap0` |
| upstream WiFi | `wlan0` |
| AP gateway | `10.88.0.1/24` |
| AP SSID | `mgate` |
| TProxy mark | `0x1` |
| TProxy route table | `100` |
| TProxy chain | `MGATE_TPROXY` |

## 4. 网络路径

普通 NAT fallback：

```text
手机 -> ap0 -> Debian IPv4 NAT -> wlan0 -> 上级 WiFi -> 公网
```

TProxy 透明代理：

```text
手机 -> ap0 -> iptables mangle/TPROXY -> mihomo tproxy-port 31802 -> TPROXY-OUT -> wlan0 -> 公网
```

## 5. 推荐采集入口

高频主采集入口：

```text
mgate agent-snapshot
```

特点：

- 只读。
- 输出 JSON。
- 包含 `wifi` / `ap` / `gateway` / `tproxy` / `web` / `subscription` / `mihomo` / `last_errors`。
- 不做 ping。
- 不 sleep。
- 不跑 doctor。
- 不启停服务。
- 不写 iptables / ip rule / ip route。
- 不修改 `config.yaml`。
- 不重启 mihomo。

建议 timeout：`2s`。

启动或低频能力刷新入口：

```text
mgate capabilities-json
```

用途：

- 判断当前 `mgate.sh` 支持哪些能力。
- 判断哪些命令是 `read_only`。
- 判断哪些命令是 `dangerous`。
- 判断哪些命令是 `interactive`。
- 获取 `agent_contract`。

建议 timeout：`2s`。

## 6. JSON 接口

当前 JSON 接口均包含：

```json
"schema_version": 1
```

现有 JSON 命令：

```text
mgate status-json
mgate wifi-json
mgate ap-json
mgate gateway-json
mgate tproxy-json
mgate agent-snapshot
mgate capabilities-json
```

agent 要求：

- 解析前先确认 JSON 合法。
- 检查 `schema_version`。
- 不依赖人类可读 `status` / `doctor` 输出做高频采集。
- 不把 `[INFO]` / `[OK]` 文本当成机器协议。
- JSON 字段缺失时要容错，不能让 agent 崩溃。

## 7. 推荐采集策略

高频采集：

```text
mgate agent-snapshot
```

推荐周期：`10s` 左右。

启动 / 能力刷新：

```text
mgate capabilities-json
```

低频或按需采集：

```text
mgate status-json
mgate wifi-json
mgate ap-json
mgate gateway-json
mgate tproxy-json
```

人工诊断或异常时采集：

```text
mgate wifi-doctor
mgate gateway-doctor
mgate tproxy-health
mgate tproxy-doctor
mgate tproxy-debug
```

`doctor` / `health` / `debug` 不适合高频采集，因为它们可能更慢，也可能包含更多诊断输出。

## 8. agent-snapshot 字段摘要

`mgate agent-snapshot` 输出包含：

- `ok`
- `schema_version`
- `component`
- `version`
- `timestamp`
- `hostname`
- `mode`
- `overall_health`
- `wifi`
- `ap`
- `gateway`
- `tproxy`
- `web`
- `subscription`
- `mihomo`
- `last_errors`
- `warnings`

`mode` 可能为：

```text
nat
tproxy
unknown
```

`overall_health` 可能为：

```text
healthy
degraded
broken
unknown
```

`last_errors` 当前可能包含：

```text
wifi
gateway
tproxy
```

没有错误时为 `null`。

## 9. capabilities-json 字段摘要

`mgate capabilities-json` 输出包含：

- `ok`
- `schema_version`
- `component`
- `version`
- `features`
- `commands`
- `agent_contract`

`features` 用于标识当前 `mgate.sh` 支持的能力，例如：

```text
mihomo
subscription
web
tui
wifi
ap
gateway
tproxy
json
preflight
```

`commands` 分为：

- `read_only`
- `dangerous`
- `interactive`

`agent_contract` 包含：

- `safe_poll_command`
- `recommended_poll_interval_seconds`
- `snapshot_timeout_seconds`
- `json_timeout_seconds`
- `doctor_timeout_seconds`
- `dangerous_actions_require_dedicated_action_api`

## 10. 当前允许调用的只读命令

高频安全：

```text
mgate agent-snapshot
```

低频安全：

```text
mgate capabilities-json
mgate status-json
mgate wifi-json
mgate ap-json
mgate gateway-json
mgate tproxy-json
```

异常诊断时可低频调用：

```text
mgate wifi-doctor
mgate gateway-doctor
mgate tproxy-health
mgate tproxy-doctor
```

诊断命令应设置更长 timeout，例如 `10-20s`。

## 11. 当前不应直接调用的命令

交互式命令：

```text
mgate tui
mgate ap-install-deps
```

危险命令：

```text
mgate wifi-connect
mgate wifi-disconnect
mgate wifi-reconnect
mgate wifi-delete
mgate ap-start
mgate ap-stop
mgate gateway-start
mgate gateway-stop
mgate tproxy-start
mgate tproxy-stop
mgate self-update
mgate update
mgate install-core
mgate sub-update
mgate web-disable
```

原因：这些命令可能导致 SSH 断线、影响 AP 客户端、修改 iptables / ip rule / ip route、修改 mihomo config、重启服务、切换上级 WiFi、触发 fallback / rollback / watchdog，或导致设备短暂失联。

## 12. 未来远程控制边界

如果 cloud 后续要远程控制 `mgate`，不能让 `mgate-agent` 执行任意 shell 命令。必须设计 agent-safe action API。

至少需要：

- 白名单动作。
- 参数校验。
- 超时控制。
- 操作锁。
- 操作审计。
- 并发保护。
- 状态上报。
- 失败回滚。
- 结果确认。
- 危险动作二次确认策略。

未来可考虑的 action：

```text
start_ap
stop_ap
start_gateway
stop_gateway
start_tproxy
stop_tproxy
switch_wifi
reconnect_wifi
update_subscription
```

这些不属于当前适配范围。当前只做状态采集和上报。

## 13. WiFi 管理状态与边界

`mgate.sh` 已实现上级 WiFi 管理命令：

```text
mgate wifi-status
mgate wifi-scan
mgate wifi-list
mgate wifi-add
mgate wifi-connect
mgate wifi-disconnect
mgate wifi-reconnect
mgate wifi-delete
mgate wifi-doctor
mgate wifi-json
```

已可作为状态采集使用：

```text
mgate wifi-status
mgate wifi-scan
mgate wifi-list
mgate wifi-doctor
mgate wifi-json
```

`mgate.sh` 不接管 `wlan0`，只通过系统现有网络管理器管理上级 WiFi 连接。

优先路径：

```text
NetworkManager / nmcli
```

降级路径：

```text
wpa_supplicant 只读降级
```

unknown 管理器：

```text
只读降级，危险操作拒绝执行
```

agent 需要知道：

- `wlan0` 是上级 WiFi。
- `ap0` 是 `mgate` 管理的 AP。
- AP / NAT / TProxy 都依赖 `wlan0` 的上游连接。
- 切换 `wlan0` 可能导致 AP 信道变化、AP 客户端短暂掉线、NAT/TProxy 暂时不可用。

## 14. mgate.sh 安全边界

`mgate.sh` 当前坚持这些边界：

- 不接管 `wlan0`。
- 不停止 NetworkManager。
- 不停止 wpa_supplicant。
- 不停止 systemd-networkd。
- 不覆盖 `/etc/hostapd/hostapd.conf`。
- 不覆盖 `/etc/dnsmasq.conf`。
- AP 使用 `/opt/mgate/run/ap/hostapd.conf`。
- dnsmasq 使用 `/opt/mgate/run/ap/dnsmasq.conf`。
- NAT fallback 必须保留。
- TProxy start 必须可回滚。
- TProxy stop 必须能回到 NAT fallback。
- Web 慢操作必须 job 化。
- CRLF 必须被 preflight / self-update / Web CGI 生成检查拦截。
- Web 只读状态页不能变成危险启停入口。

`mgate-agent` 适配时不能破坏这些边界。

## 15. 适配路线建议

只读状态采集：

1. 启动时调用 `mgate capabilities-json`。
2. 定时调用 `mgate agent-snapshot`。
3. 解析 `schema_version`。
4. 上报 cloud。
5. cloud 展示 wifi / ap / gateway / tproxy / mihomo / subscription / web 状态。
6. 命令超时后上报 `snapshot_timeout`。
7. 命令不可用时上报 `mgate_unavailable`。
8. 不做远程控制。

后续诊断增强：

1. 增加低频 doctor 采样。
2. 异常时自动采集 `tproxy-debug` 或 `gateway-doctor`。
3. cloud 增加诊断详情页。
4. 仍然不做危险动作。

未来远程控制：

1. 设计 agent-safe action API。
2. 将危险动作纳入专用 action、操作锁、审计、回滚和二次确认策略。

## 16. 建议 timeout

| 命令类型 | 建议 timeout |
| --- | ---: |
| `agent-snapshot` | `2s` |
| `capabilities-json` | `2s` |
| 单独 `*-json` | `2s` |
| `doctor` / `health` / `debug` | `10-20s` |
| 危险 action | 后续另行设计 |

## 17. 异常上报建议

agent 至少要区分：

```text
mgate_missing
mgate_permission_denied
snapshot_timeout
snapshot_invalid_json
snapshot_schema_unsupported
snapshot_ok
snapshot_degraded
snapshot_broken
```

不要只报 generic error。否则 cloud 侧会像雾里看花，漂亮但没用。
