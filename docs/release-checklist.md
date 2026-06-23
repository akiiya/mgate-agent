# v0.1.0-rc1 Tag 前清单

发布前逐项确认。最后一项真机验收需要人工在设备上完成。

## 必过门禁

- [ ] gofmt 检查通过
- [ ] `go test ./...` 通过
- [ ] `go vet ./...` 通过
- [ ] 安全门禁测试通过
- [ ] fake cloud smoke test 通过
- [ ] Linux amd64 构建通过
- [ ] Linux arm64 构建通过
- [ ] Linux armv7 构建通过
- [ ] `make release VERSION=v0.1.0-rc1` 通过
- [ ] `make verify-release VERSION=v0.1.0-rc1` 通过
- [ ] `dist/checksums.txt` 已生成
- [ ] `cd dist && sha256sum -c checksums.txt` 通过
- [ ] release tar.gz 可解压
- [ ] release 包名包含完整 tag：`v0.1.0-rc1`
- [ ] release 包不包含真实 credentials
- [ ] release 包不包含 `.git`、outbox 测试数据或本地 `bin/` 临时产物
- [ ] `mgate-agent version` 正常
- [ ] `mgate-agent config default` 正常
- [ ] `mgate-agent check` 正常
- [ ] `mgate-agent doctor` 不泄露 secret
- [ ] `docs/device-acceptance.md` 已更新
- [ ] `docs/release-notes/v0.1.0-rc1.md` 已更新
- [ ] tag 触发的 `Release Artifacts` workflow 通过
- [ ] GitHub Release 已上传三个 tar.gz 和 `checksums.txt`
- [ ] 已在至少一台测试设备上完成部署验收

## 本地命令

```sh
gofmt -w .
go test ./...
go vet ./...
go build -o bin/mgate-agent ./cmd/mgate-agent
go test ./internal/integration -count=1
```

## Linux 构建

```sh
GOOS=linux GOARCH=amd64 go build -o bin/mgate-agent-linux-amd64 ./cmd/mgate-agent
GOOS=linux GOARCH=arm64 go build -o bin/mgate-agent-linux-arm64 ./cmd/mgate-agent
GOOS=linux GOARCH=arm GOARM=7 go build -o bin/mgate-agent-linux-armv7 ./cmd/mgate-agent
```

## Release 包

```sh
make release VERSION=v0.1.0-rc1
make verify-release VERSION=v0.1.0-rc1
```

确认生成：

```text
dist/mgate-agent-v0.1.0-rc1-linux-amd64.tar.gz
dist/mgate-agent-v0.1.0-rc1-linux-arm64.tar.gz
dist/mgate-agent-v0.1.0-rc1-linux-armv7.tar.gz
dist/checksums.txt
```

每个包应包含顶层目录，例如：

```text
mgate-agent-v0.1.0-rc1-linux-armv7/
```

校验 SHA256：

```sh
cd dist
sha256sum -c checksums.txt
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

示例：

```text
mgate-agent-v0.1.0-rc1-linux-armv7.tar.gz
```

架构映射：

| 设备架构 | Release 包 |
| --- | --- |
| `x86_64` | `linux-amd64` |
| `aarch64` | `linux-arm64` |
| `armv7l` | `linux-armv7` |

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
10. 模拟 result 回传失败，确认 outbox pending 增加。
11. 恢复连接，确认 outbox pending 下降。
12. 确认本地 command 没有因为补发 result 而重新执行。

## 不应出现

- enroll 功能。
- mgate-cloud 服务端实现。
- command 持久化或重放。
- `result_ack` 强确认。
- 新增远程 action。
- AP/TProxy/wlan/mihomo 业务逻辑重写。
