# 发布前审计

本清单用于发布前确认 `mgate-agent` 的安全边界、发布产物和部署脚本仍然符合项目约束。

## 1. 远程执行边界

确认生产链路仍然是：

```text
WebSocket / HTTPS Pull
  -> internal/transport
    -> internal/commands.Handler
      -> internal/actions
      -> internal/runner
        -> mgate.sh argv
```

检查项：

- transport 不直接调用 runner。
- transport 不直接调用 action registry。
- cloud 不能下发任意命令行。
- action 只能来自硬编码 registry。
- `security.allow_actions` 仍是本机二次收窄。
- runner 只使用 `exec.CommandContext(ctx, mgatePath, argv...)`。
- 不存在 `sh -c` 或 `bash -c`。

## 2. mgate.sh 只读采集边界

确认 `internal/mgate` 只调用：

```text
mgate capabilities-json
mgate agent-snapshot
```

检查项：

- 不调用 `wifi-connect`、`gateway-start`、`tproxy-start`、`ap-start` 等控制命令。
- 采集失败只上报 `mgate.available=false` 和稳定 `error_code`。
- 采集失败不影响 WebSocket、Pull、outbox 或 command handler。
- mgate stdout/stderr 不原样写入普通日志。

## 3. 敏感信息边界

确认：

- `device_secret` 不进入日志、audit、doctor、outbox 元数据。
- signature 不进入 URL。
- psk/token/password 有基础脱敏。
- audit 不记录 stdout/stderr 全量内容。
- Release 包不包含真实 credentials。

## 4. outbox 边界

确认：

- outbox 只保存 result envelope。
- outbox 不保存 command。
- result 发送失败不会导致 command 重放。
- outbox 有数量上限和大小上限。
- 损坏文件不会导致 agent 崩溃。

## 5. 发布产物

每个 Release 包应包含：

```text
mgate-agent
configs/agent.example.yaml
packaging/systemd/mgate-agent.service
scripts/install.sh
scripts/uninstall.sh
README.md
LICENSE
docs/
```

不应包含：

- `.git`
- credentials
- outbox 测试数据
- 本地临时文件
- 真实 secret

## 6. 发布流程

发布由 GitHub Release 页面触发。用户手动创建 Release 并填写全新 tag 后，`Release Assets` workflow 负责测试、构建、打包、校验 checksum 并上传 assets。

仓库不维护 `VERSION` 文件，也不维护 `docs/release-notes/`。Release notes 直接写在 GitHub Release 页面中。
