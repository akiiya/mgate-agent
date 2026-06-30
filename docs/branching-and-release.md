# 分支与发布流程

`mgate-agent` 使用 `dev` / `main` 双分支与 GitHub Release 事件发布模型。

## dev 分支

`dev` 是验证平台。推送到 `dev` 或创建到 `main` 的 PR 后，`Dev Verification` workflow 只做代码检查：

- gofmt 检查。
- `go vet ./...`。
- `go test ./...`。

`dev` workflow 不编译 release 包，不运行 `make release`，不创建 tag，不创建 GitHub Release，也不上传 release assets。

## main 分支

`main` 是发布窗口，只应通过 GitHub 页面从已验证分支 merge 进入。

代码进入 `main` 后不会自动发布。发布由用户在 GitHub 项目页面手动创建 Release 开始：

1. 在 GitHub 项目页面创建新的 Release。
2. 填写一个全新的 tag，格式为 `vMAJOR.MINOR.PATCH[-PRERELEASE]`。
3. 在 Release 页面直接填写 release notes。
4. 发布 Release。
5. `Release Assets` workflow 监听 `release.published` 事件。
6. workflow 使用 Release tag 作为版本号。
7. workflow 执行 gofmt、vet、test、host build 和 Linux 多架构构建。
8. workflow 执行 `make release VERSION=<tag>`。
9. workflow 执行 `make verify-release VERSION=<tag>`。
10. workflow 校验 `dist/checksums.txt`。
11. workflow 上传三个 tar.gz 和 `checksums.txt` 到刚创建的 GitHub Release。

## 版本来源

版本号只来自 GitHub Release tag。仓库根目录不维护 `VERSION` 文件，workflow 也不从仓库文件读取版本。

tag 格式要求：

```text
^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$
```

本地手工打包时必须显式传入 tag：

```sh
make release VERSION=<tag>
make verify-release VERSION=<tag>
```

## Release Notes

仓库不维护 `docs/release-notes/`。

每次发布的新内容、风险说明、升级说明和已知限制，直接写在 GitHub Release 页面中。这样 release notes 与对应 tag 绑定，不会让仓库文档残留过期版本说明。

## 重发策略

不覆盖旧 Release assets。如果同名资产已经存在，workflow 会失败。

需要重发 RC 或修复错误发布时，推荐发布一个新 tag。只有在明确需要清理错误发布时，才人工删除旧 Release/tag 后重试。

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
