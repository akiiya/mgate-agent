# 发布检查清单

发布前逐项确认。最后的真机验收需要人工在设备上完成。

## 代码检查

- [ ] `dev` 已包含待发布代码。
- [ ] `Dev Verification` workflow 通过。
- [ ] gofmt 检查通过。
- [ ] `go test ./...` 通过。
- [ ] `go vet ./...` 通过。
- [ ] 安全门禁测试通过。
- [ ] fake cloud smoke test 通过。
- [ ] 已通过 GitHub 页面将 `dev` merge 到 `main`。

## Release 创建

- [ ] 在 GitHub 项目页面手动创建 Release。
- [ ] 使用全新 tag，格式为 `vMAJOR.MINOR.PATCH[-PRERELEASE]`。
- [ ] release notes 已直接填写在 GitHub Release 页面。
- [ ] RC / beta / alpha 版本已在 GitHub Release 页面手动勾选 pre-release。

仓库不维护 `docs/release-notes/`，不要把 release notes 作为常规文档长期保留。

## Release Assets workflow

Release 发布后，`Release Assets` workflow 应自动执行：

- [ ] gofmt 检查通过。
- [ ] `go vet ./...` 通过。
- [ ] `go test ./...` 通过。
- [ ] host build 通过。
- [ ] Linux amd64 构建通过。
- [ ] Linux arm64 构建通过。
- [ ] Linux armv7 构建通过。
- [ ] `make release VERSION=<tag>` 通过。
- [ ] `make verify-release VERSION=<tag>` 通过。
- [ ] `cd dist && sha256sum -c checksums.txt` 通过。
- [ ] GitHub Release 已上传三个 tar.gz 和 `checksums.txt`。

## Release 包

本地需要复现时，必须显式传入 tag：

```sh
make release VERSION=<tag>
make verify-release VERSION=<tag>
cd dist
sha256sum -c checksums.txt
```

应生成：

```text
dist/mgate-agent-<tag>-linux-amd64.tar.gz
dist/mgate-agent-<tag>-linux-arm64.tar.gz
dist/mgate-agent-<tag>-linux-armv7.tar.gz
dist/checksums.txt
```

每个包应包含顶层目录，例如：

```text
mgate-agent-<tag>-linux-armv7/
```

目录内应包含：

- `mgate-agent`
- `configs/agent.example.yaml`
- `packaging/systemd/mgate-agent.service`
- `scripts/install.sh`
- `scripts/uninstall.sh`
- `docs/`
- `README.md`
- `LICENSE`

不应包含：

- `.git`
- credentials
- outbox 测试数据
- 本地临时文件
- 真实 secret

## Release asset 命名规则

```text
mgate-agent-<tag>-linux-amd64.tar.gz
mgate-agent-<tag>-linux-arm64.tar.gz
mgate-agent-<tag>-linux-armv7.tar.gz
checksums.txt
```

架构映射：

| 设备架构 | Release 包 |
| --- | --- |
| `x86_64` | `linux-amd64` |
| `aarch64` | `linux-arm64` |
| `armv7l` | `linux-armv7` |

## 重发策略

- 不覆盖已有 Release assets。
- 如果资产已存在，workflow 会失败。
- 推荐发布新 tag。
- 只有明确需要清理错误发布时，才人工删除旧 Release/tag 后重试。

## 设备手工验证

1. 上传对应架构 release 包。
2. 解压后执行 `scripts/install.sh`。
3. 准备 `/var/lib/mgate-agent/credentials.json`。
4. 执行 `mgate-agent check`。
5. 执行 `mgate-agent doctor`。
6. 启动 systemd。
7. 查看 `journalctl -u mgate-agent -f`。
8. 验证 WebSocket command -> result。
9. 阻断 WebSocket，验证 Pull command -> result POST。
10. 验证 `mgate capabilities-json` 与 `mgate agent-snapshot` 只读状态采集。
11. 模拟 result 回传失败，确认 outbox pending 增加。
12. 恢复连接，确认 outbox pending 下降。
13. 确认本地 command 没有因为补发 result 而重新执行。

## 不应出现

- enroll 功能。
- mgate-cloud 服务端实现。
- command 持久化或重放。
- `result_ack` 强确认。
- 新增远程控制 action。
- AP/TProxy/wlan/mihomo 业务逻辑重写。
