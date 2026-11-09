# 贡献指南

欢迎提 Issue 与 PR。这是一个刻意在能力上做减法的工具，贡献前请先读完这一页。

## 不可协商的红线

**任何改动都不得引入**：按键序列 / 字符文本 / 窗口标题 / 进程名 / 剪贴板 / 截图的采集，原始事件时序落盘，对外联网、遥测、上传，把统计数据暴露给本机之外的访问，以及隐藏进程、注入、写开机启动项等行为。

全局键盘钩子 + 本地落盘已经很接近恶意软件的行为特征，唯一让它站得住脚的就是「只记次数、代码可自查」。加壳、混淆、绕过杀软检测的 PR 一律拒绝。若要增加统计维度，先确认它能否用**聚合计数**表达；不能，就别做。

## 开发环境

- Windows 10/11 x64
- Go 1.25+（`CGO_ENABLED=0`，零第三方依赖，不需要 gcc）
- Node.js 18+（仅用于报告页面渲染回归）
- Git Bash 或等价 POSIX shell

默认使用 `PATH` 里的 `go`。若用便携版 Go，复制 `.env.local.example` 为 `.env.local` 并填写 `KP_GO_BIN` / `KP_GOROOT` / `KP_GOCACHE`；该文件被 `.gitignore` 排除。

## 常用命令

```bash
bash build.sh                                  # 构建两个 exe
go test -buildvcs=false -count=1 ./...          # Go 单测（含 Win32 ABI 冒烟测试）
./keypulse-cli.exe --selftest                   # 生成 35 天模拟数据与示例报告
node scripts/check-page.js .tmp/_selftest/report.html   # 报告三视图渲染回归
bash scripts/check-version.sh                  # 发版前版本校验
bash scripts/package-release.sh                # 完整发布包
```

`go build` 需要 `-buildvcs=false`（构建脚本已处理）。`go vet` 对 Win32 `syscall` 传指针会报 `possible misuse of unsafe.Pointer`，这是固有写法，已有 ABI 冒烟测试覆盖，不作为阻断条件。

## 代码要求

- 遵循 `AGENTS.md` 的铁律：注释只写「为什么」，不复述「是什么」；只改当前功能范围内的代码。
- **钩子回调里禁止**：分配内存、加锁、调用 `time.Now()`、发 channel、碰文件、碰 UI。回调超时会被系统静默卸载钩子。
- 新增统计口径必须同步改 `docs/metrics.md`，新增报告字段必须同步改 `scripts/check-page.js` 的回归断言。
- 无第三方依赖是设计约束，不是现状描述；引入依赖前先讨论。
- 行为变更要同步更新 `README.md` 与 `CHANGELOG.md` 的 `Unreleased` 区。

## 提交与 PR

- 提交信息用**中文**，格式 `<type>: <简述>`；`type` 取 `feat` / `fix` / `perf` / `refactor` / `docs` / `test` / `build` / `release`。
- 单次提交对应一个独立功能点，不攒大包。
- 提交前检查暂存区：不得含 Token、密码、本机绝对路径、`.env.local`、`*.bin`、真实 `report.html`。
- PR 描述写明：改动动机、验证命令与结果、是否影响统计口径或数据格式。

## 版本

版本号由维护者按 [`docs/versioning.md`](docs/versioning.md) 判定，PR 里不要自行修改 `VERSION` 或打 tag。`1.x` 长期演进，新功能走 `1.x.0`、修复走 `1.1.x`，只有破坏兼容且无法迁移才升 `2.0.0`。

## 报告问题

提 Issue 时附上 `keypulse-cli.exe --version` 的输出、复现步骤、以及 `health.log` 是否存在（**不要**粘贴 `.bin` 或 `export.csv` 内容）。
