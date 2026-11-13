# 开发与发布

## 工具与本地运行

使用 Node.js 24、`.go-version` 指定的 Go 1.27.4、Python 3。依赖由 `package-lock.json` 与 `go.sum` 锁定。

```sh
npm ci
npm run build
sh scripts/dev.sh
```

演示服务仅监听 `127.0.0.1:18946`，在 `.local/data/` 创建测试目录与执行记录。开发前端时保持后端运行，在另一个终端执行 `npm run dev`；Vite 会转发业务 API。

## 验证与打包

```sh
python3 scripts/check-metadata.py
npm test
npm run build
go test -race ./server
go vet ./server
python3 scripts/test-install.py
python3 scripts/test-release.py
python3 scripts/build-package.py amd64
python3 scripts/build-package.py arm64
python3 scripts/verify-package.py build/batch-rename_0.1.0_amd64.deb build/batch-rename_0.1.0_arm64.deb
python3 scripts/check-security.py build/batch-rename_0.1.0_amd64.deb build/batch-rename_0.1.0_arm64.deb
```

打包脚本先核对 `.go-version`，禁用自动切换 Go 工具链，再校验元数据并重新构建前端，再交叉编译静态 Linux ELF，生成 deb，附带工具链版本、架构和二进制 SHA-256 的 `build-info.json`。安全检查使用 `.govulncheck-version` 固定的官方工具，扫描 npm 锁定依赖、两个 Linux 架构的后端源码及指定包内的实际二进制。可用 `--go /path/to/go` 指定 Go 工具链。打包可在 macOS 运行，安装包结构校验会比对源码元数据、当前前端构建和许可证。

在 Linux 中还需使用[官方自测工具](https://github.com/terramaster-tos/tos-app-pkg-tools/releases/tag/v1.0.3-beta)分别检查两个 deb。GitHub Actions 固定工具版本和 SHA-256，在上传产物前执行此检查。

## 自动化

`.github/workflows/release.yml` 在 `main` 推送、拉取请求和手动运行时执行验证，并上传双架构构建产物；推送 `v*` 标签时，在所有检查成功后创建 GitHub Release。

- CI 测试在 Ubuntu 24.04 运行，包含后端竞态检测。
- Ubuntu 构建任务安装 amd64 deb，以真实 systemd 验证账号延后创建、启动前权限初始化、非 root 业务进程、故障恢复、升级及卸载；此检查不能代替 TOS 真机验收。
- Linux 构建两个架构，执行依赖与二进制漏洞扫描、项目校验和官方包自测，生成 `SHA256SUMS`。
- 只有标签发布任务具有 `contents: write` 权限，使用仓库内置 `GITHUB_TOKEN`。
- 标签必须与源码版本一致，例如版本 `0.1.0` 对应 `v0.1.0`。
- `packaging/config.ini` 的 `beta: true` 对应 GitHub 预发布；`false` 对应正式发布。
- 发布附件包含两个 deb 及校验和；同一标签的已有 Release 不会被覆盖。

创建 GitHub 仓库时使用 `LuYifei2011/tos-batch-rename-tool`，默认分支为 `main`。仓库名称变化时，同步修改 `package.json` 中的 repository、homepage、bugs 和 `packaging/config.ini` 中的 help。

## 发布版本

1. 同步 `package.json`、`package-lock.json` 和 `packaging/config.ini` 的三段数字版本号，更新 README、安装语言文件及 `docs/RELEASE_NOTES.md`。
2. 完成本地验证、依赖安全检查及[最终安装包的 NAS 验收](COMPATIBILITY.md)。自动化检查不能代替真实设备的写入验收。
3. 提交并推送到 `main`，检查 CI 通过。
4. 为待发布提交创建并推送签名标签：

```sh
git tag -s v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
```

推送标签即触发自动发布。升级版本时使用对应的新标签；修正已发布版本应递增版本号。

## 项目文件

- `src/`：Vue 界面、改名规则和预览 Worker。
- `server/`：会话、目录访问、执行计划及日志。
- `packaging/`：TOS 元数据、systemd 单元和 deb 生命周期脚本。
- `scripts/`：开发、构建和验证命令。
- `build/`、`dist/`：构建产物，不纳入版本控制。
- `.local/`：本地演示运行数据，不纳入版本控制。

当前界面规范见 [DESIGN.md](DESIGN.md)。
