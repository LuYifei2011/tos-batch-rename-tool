# 后端可执行文件来源

包内的 `/usr/local/batch-rename/bin/batch-rename` 是本项目的 Go 后端，由项目维护者从仓库的 `server/` 源码编译。它通过 systemd Unix socket 接收前端请求，调用本机 TOS 文件服务校验会话、预览和执行改名，并在 NAS 上保存任务记录以支持撤销。systemd 启动前也使用此文件的 `-prepare-data` 模式初始化应用数据目录。

公开发布包的源码位于同版本的 Git 标签 `v<版本号>`。从对应 Release 打开标签即可查阅 `server/`、依赖清单 `go.mod` 和 `go.sum`、Go 版本 `.go-version`、构建脚本 `scripts/build-package.py` 及发布工作流 `.github/workflows/release.yml`。[项目发布与源码标签](https://github.com/LuYifei2011/tos-batch-rename-tool/releases)

CI 按目标架构分别设置 `GOOS=linux`、`GOARCH=amd64` 或 `arm64`、`CGO_ENABLED=0`，从 `./server` 编译。包内同一应用目录的 `build-info.json` 记录该次构建的 Go 版本、目标架构和可执行文件的 SHA-256。发布页的 `SHA256SUMS` 用于核对完整安装包。依赖的授权说明随包放在 `THIRD_PARTY_NOTICES.md` 和 `licenses/`。

审核时可在解包后的应用目录执行以下命令，对照 `binarySha256` 并查看 Go 模块构建信息。

```sh
sha256sum bin/batch-rename
go version -m bin/batch-rename
```
