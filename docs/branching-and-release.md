# 分支与发布流程

`mgate-agent` 使用 dev/main 双分支发布模型。

## dev 分支

`dev` 是验证平台。

推送到 `dev` 或创建到 `main` 的 PR 后，`Dev Verification` workflow 会执行：

- 校验 `VERSION`。
- gofmt 检查。
- `go vet ./...`。
- `go test ./...`。
- host build。
- Linux amd64 / arm64 / armv7 build。
- `make release`。
- `make verify-release`。
- `sha256sum -c checksums.txt`。

`dev` workflow 只做验证：

- 不创建 tag。
- 不创建 GitHub Release。
- 不上传 release assets。

## main 分支

`main` 是发布窗口，只应通过 GitHub 页面从已验证分支 merge 进入。

代码进入 `main` 后，`Main Release` workflow 会：

1. 读取仓库根目录 `VERSION`。
2. 校验版本号格式。
3. 检查远端 tag 是否已存在。
4. 检查 GitHub Release 是否已存在。
5. 执行 gofmt、vet、test 和 build。
6. 执行 `make release VERSION=<VERSION>`。
7. 执行 `make verify-release VERSION=<VERSION>`。
8. 校验 `dist/checksums.txt`。
9. 创建 annotated tag。
10. 创建 GitHub Release。
11. 上传三个 tar.gz 与 `checksums.txt`。

如果 tag 或 Release 已存在，workflow 会失败，不会覆盖旧资产。

## VERSION 文件

`VERSION` 是唯一默认版本来源。

示例：

```text
v0.1.0
v0.1.0-rc1
v0.1.0-beta1
v0.1.0-alpha1
```

格式要求：

```text
^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$
```

发布新版本时，只修改 `VERSION` 文件。`Makefile` 允许临时覆盖 `VERSION=...`，但 CI/CD 默认都读取仓库中的 `VERSION`。

## 推荐发布流程

1. 修改代码和 `VERSION`。
2. 推送到 `dev`。
3. 等待 `Dev Verification` workflow 通过。
4. 人工确认 release notes、文档和真机验收准备。
5. 在 GitHub 页面将 `dev` merge 到 `main`。
6. `Main Release` workflow 自动发布。

## 重发版本策略

不覆盖旧 Release，不 force push tag。

如果需要重发 RC，请发布新版本，例如：

```text
v0.1.0-rc2
```

只有在明确需要清理错误发布时，才人工删除旧 Release/tag 后重试。

## 外部安装器

外部安装器应消费 GitHub Release assets，而不是 Actions artifact。

稳定资产命名：

```text
mgate-agent-<tag>-linux-amd64.tar.gz
mgate-agent-<tag>-linux-arm64.tar.gz
mgate-agent-<tag>-linux-armv7.tar.gz
checksums.txt
```

安装器应下载对应架构包，并使用 `checksums.txt` 校验 SHA256。
