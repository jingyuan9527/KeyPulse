# TODOS

## v1.0.0 发布收尾（2026-11-06）

- [x] 版本号：VERSION / internal/version / `--version` / 构建注入 / 报告与托盘可追溯
- [x] 托盘图标按 `SM_CXSMICON` 实际尺寸绘制（16/20/24/32/48），小尺寸精简键帽列数
- [x] 睡眠/休眠/VM 暂停：墙上时间跳跃检测 → 强制 `Flush(true)` + 立即触发钩子心跳
- [x] 报告生成失败时明确输出原因（不再静默吞错）
- [x] `docs/release.md` 发布与自测清单、Defender 处理步骤、已知限制、回滚
- [x] README / docs/metrics.md 与代码行为对齐
- [x] 生成 `dist/KeyPulse-1.0.0/` 发布包（两版 exe、文档、示例报告、SHA256SUMS）
- [x] 最终全量验证：`go test ./...`、页面回归、`--version`、PE subsystem、7 项校验和全 OK
- [x] 用户退出 PID 3468 后，根目录 `keypulse.exe` 已替换为 1.0.0 正式版（SHA256 与 `dist/` 一致）；`--version`、`--report-only` 冒烟通过。托盘常驻需用户在交互桌面双击启动：工具会话派生的进程会被作业对象回收（`CREATE_BREAKAWAY_FROM_JOB` 返回拒绝访问），无法代为常驻

## 开源发布与版本治理（2026-11-06）

- [x] 重写 `README.md` 面向公开仓库：隐私声明前置、三视图、口径摘要、版本规则、开发命令
- [x] `docs/versioning.md`：SemVer 升位决策树、**主版本克制清单**、v1 兼容承诺、13 步发版 SOP、tag / Release / 资产命名
- [x] `CHANGELOG.md`：Keep a Changelog 中文版，含 1.0.0 完整条目与常驻 Unreleased
- [x] `scripts/check-version.sh`：VERSION 格式 / CHANGELOG 条目 / tag 重复 / 文档写死版本号校验
- [x] 构建脚本去掉本机硬编码路径，改为 `.env.local` 可选覆盖；新增 `.env.local.example`
- [x] `.gitignore` / `CONTRIBUTING.md`（含不可协商的隐私红线）
- [x] `docs/metrics.md` / `docs/release.md` 去掉写死版本号；release 新增第 10 节 GitHub Release 规范
- [x] `scripts/package-release.sh` 增加版本校验、zip 打包、动态 SHA256 清单
- [x] 公开仓库 `jingyuan9527/KeyPulse`（MIT、topics、`main` 分支）：4 个中文提交 + `v1.0.0` annotated tag，提交身份用 GitHub noreply 邮箱
- [x] GitHub Release `v1.0.0 · 首个稳定版本`（latest）：上传 `KeyPulse-1.0.0-win-x64.zip` 与 `SHA256SUMS.txt`，正文含变更、校验、隐私边界与 Defender 说明
- [x] 重跑 `package-release.sh`：10 项 SHA256 全 OK、页面回归 7 项 PASS、zip 2.20 MB、两版 exe subsystem 正确
- [x] 提交前隐私扫描：4 组暂存区均无本机路径 / 真实邮箱 / Token；`.env.local`、`dist/`、exe、`.workbuddy/`、`.tmp/` 均已忽略
- [x] `.gitattributes` 固定 LF，避免 Windows `core.autocrlf` 把 `.sh` 转成 CRLF 导致 Git Bash 构建失败

## 已交付的 v1 能力

- [x] 双低层钩子 + 独立 OS 线程消息循环 + 回调零分配
- [x] 按天 1536 B 二进制快照 + 内存脏位图增量写盘 + 空闲零 IO
- [x] 托盘常驻、程序化两态图标、暂停/导出/退出
- [x] 离线单文件 HTML 报告：单日回看 / 周报同期对比 / 总览日历热力
- [x] 今日速写（数据驱动文案）、托盘图标、报告 10 分钟保活自动刷新
- [x] P0 钩子心跳检测与自动重挂、`health.log` 数据缺口标注

## v1.0.1 发布（2026-11-06）

- [x] `VERSION` → 1.0.1；`CHANGELOG.md` 增 1.0.1 条目（如实写明仅交付形态变化、程序功能与 1.0.0 相同、不含 macOS）
- [x] `scripts/package-release.sh` 同时产出免解压直下的 `KeyPulse-1.0.1-win-x64.exe`，并打印上传资产 SHA256
- [x] `scripts/check-version.sh`、`docs/versioning.md` §7/§8、`docs/release.md`、`README.md` 同步三资产规范
- [ ] 全量构建与校验、本地提交、推送 tag、创建 Release 并上传 exe / zip / SHA256SUMS 三个资产、匿名回读校验

## 后续 PATCH 候选（本轮未改代码）

- [ ] `health.log` 出现 3 条 RECOVER（2026-11-06 11:51:01 / 11:52:43 / 13:00:51，gap 1~2s，reason=input_without_callback），推测集中在锁屏切换与退出托盘瞬间：系统最后输入 tick 变化而钩子已无回调，心跳按保守策略重挂并标注近似缺口。数据本身未丢失，属「宁可提示近似也不漏计数」的设计结果，不是故障；若确认主要来自退出路径，可在 `h.Quit()` 前置 stopping 标志抑制最后一轮心跳。任何改动按 `docs/versioning.md` 以新的 PATCH 发布（1.0.1 已用于交付形态调整），不得跳版本。

## macOS 移植（后续新能力，需 `1.x.0`）

- [ ] 前置条件：需要 Mac 或 GitHub Actions macOS runner；签名与公证需要 Apple Developer ID
- [ ] 平台抽象：`main.go` 的 `CreateMutexW` 单实例、`%LOCALAPPDATA%` 数据目录、`ShellExecuteW` 打开报告按 GOOS 拆文件
- [ ] 采集层：`CGEventTap` + 输入监控权限引导，回调仍只做原子计数；Mac 侧必须 cgo（破「零 CGO」，保「零第三方依赖」）
- [ ] 菜单栏层：`NSStatusItem` + `LSUIElement` 的 `.app` bundle、模板图标、暂停 / 导出 / 退出
- [ ] 构建与分发：universal `.dmg`、CI、Gatekeeper 说明；README / release.md / versioning.md 双平台化
- [ ] 实机验证：输入监控授权、菜单栏图标、休眠恢复（CI 只能验证能编译）
- [x] 本轮决策（2026-11-06）：用户暂无 Mac / 无签名条件，**不移植、不发空壳包**；先按 1.0.1 发 Windows 直下 exe

## v2 待办

- [ ] Wails 原生窗口（当前借用系统默认浏览器展示 HTML）
- [ ] 长按连发是否计入的设置开关（v1 固定过滤）
- [ ] 组合键白名单计数（Ctrl+C / Alt+Tab 等，只存计数不存序列）
- [ ] 开机自启选项（v1 不写任何注册表启动项）
- [ ] 16×16 图标实机目视复核（当前按系统尺寸绘制，已完成 DPI 尺寸适配）
- [ ] 完整托盘/钩子链路的自动化实机回归（当前依赖人工自测清单）
