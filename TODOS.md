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
- [x] 全量构建与校验、本地提交、推送 tag、创建 Release 并上传 exe / zip / SHA256SUMS 三个资产、回读校验（`go test ./...` 全 PASS、页面回归 7 项 PASS、PE subsystem 正确、10 项清单 OK、三个资产服务端 digest 与回读字节哈希一致，latest 已指向 v1.0.1）
- [x] 2026-11-09 用户退出托盘后已替换本机根目录 exe；为同时验证实时模式，`keypulse.exe` / `keypulse-cli.exe` / `keypulse-live-preview.exe` 统一换成本地预览构建（含实时模式，`VERSION` 仍 1.0.1），**不是** 1.0.1 正式版。线上 `v1.0.1` Release 与 `dist/KeyPulse-1.0.1-win-x64.exe` 保持原样，历史数据不清空。

## 实时模式（2026-11-06 本轮实现，待发 MINOR）

方案选型：用户确认走 **B（file:// 本地 live.js + 页面局部更新）**，否决 A（1 秒整页重载会闪）与 C（本地 HTTP/SSE 会碰「零联网」红线、按 `docs/versioning.md` §3.2 可能要升 2.0.0）。

- [x] `internal/report/live.go`：`WriteLive` 读当天 + 前一日 `.bin`，复用 `toDayPayload` 生成与报告同构的今日快照；`RemoveLive` 关闭时清理；写文件加 `liveMu`
- [x] `internal/report/report.go`：`RenderWithLive` 注入 `__KP_LIVE__` 模式标志；`Render` 保持原签名（普通模式）
- [x] `internal/report/page.go`：动态 `<script src="live.js?t=...">` 每秒轮询，`applyLive` 替换今日数据后复用 `renderDay` 局部重渲染；只在今日视图更新；焦点在下拉框时跳过本次；连续 5 次加载失败才提示
- [x] `cmd/keypulse/main.go`：`liveMode` 原子开关（默认关、重启不恢复）、`liveSeq`；ticker 在 `Flush` 后且仅 `changed/force` 时写 `live.js`；开关切换的文件 IO 全丢 goroutine，不阻塞钩子消息循环；切换时先 `Flush(true)` 防数字倒退
- [x] `internal/tray/tray_windows.go`：`MF_CHECKED` 勾选菜单项、`OnLiveToggle` / `Live` 回调、切换气泡提示
- [x] 测试：`internal/report/live_test.go`（口径一致、seq 递增、清理、缺文件、模式标志）；`scripts/check-page.js` 新增实时局部更新与普通模式零轮询两项
- [x] 验证：`go test ./...` 全 PASS、页面回归 **9 项 PASS**、`gofmt` 干净、`go vet` 仅剩已知 3 条 Win32 `unsafe.Pointer` 提示
- [x] **实机验证缓存行为**：Chrome headless + CDP 确认 `file://` 下带 query 的动态 script 能读到改写后的内容（seq 1→2→3），B 方案的关键前提成立
- [x] 文档：`README.md` 新增「实时模式（可选）」小节与落盘清单、`CHANGELOG.md` Unreleased 条目
- [x] 2026-11-09 重新打包本地预览版：`dist/KeyPulse-live-preview/`（两版 exe + 文档 + 示例报告 + `LIVE-PREVIEW.txt` + 11 项 `SHA256SUMS.txt`）、`dist/KeyPulse-live-preview-win-x64.exe`、`dist/KeyPulse-live-preview-win-x64.zip`；根目录三个 exe 同步替换。验证：`go test ./...` 全 PASS、页面回归 9 项 PASS、`--version` = `1.0.1 (build 2026-11-09T02:27:12Z)`、PE subsystem 2/3 正确、11 项清单 OK。**未升 VERSION、未推送、未发版**
- [x] 用户于 2026-11-09 确认「两个功能一起发 **MINOR 1.1.0**」；按 `docs/versioning.md` §4 新增的合并发版例外条款执行，CHANGELOG 分节登记、Release 正文逐项说明。发布执行项见下方「v1.1.0 发布」小节
- [ ] 人工实机复核（工具会话无法常驻托盘，只能由用户在交互桌面完成）：托盘菜单勾选状态、切换气泡、报告页数字随按键跳动、关闭后 `live.js` 被删除并恢复 60 秒刷新

## 后续 PATCH 候选（本轮未改代码）

- [x] **已修复（2026-11-09）**：根因不是「锁屏误判」，而是 `GetLastInputInfo` 的 `dwTime` 更新滞后于低层钩子回调一个心跳周期，旧 `decideAction` 只看单周期「tick 变化 + 零回调」就重挂。67 条记录里 42 条 `gap<=1s`（掉线前 1 秒钩子还在回调）即为铁证。修复：`internal/hook/hook_windows.go` 加 30 秒连续静默确认窗口、`inputSinceAlive` 跨周期累计、重挂成功后重置观察窗口；`internal/report/data.go` 只统计新版 `reason=confirmed_silence`，旧误报行保留但不再挂提示；托盘气泡文案同步。新增 3 组单测（判定全表、跨周期时序错位、恢复窗口复位）与 1 个报告端旧日志忽略用例。**未删任何用户数据**，`health.log` 原 67 行保持原样。
- [x] 发版顺序已由用户拍板：不与实时模式拆版，按 `docs/versioning.md` §4 新增的「合并发版」例外条款随 **1.1.0** 一起发布；CHANGELOG 分节登记、Release 正文逐项说明，未夹带第二个新能力。

## v1.1.0 发布（2026-11-09）

合并发版：实时模式（新能力）+ `health.log` 假阳性修复（随版 PATCH 性质修复）。

- [x] 升 `VERSION` → 1.1.0；`CHANGELOG.md` 把 Unreleased 归入 `[1.1.0] - 2026-11-09` 并保留常驻空 Unreleased、补 compare 链接
- [x] `docs/versioning.md` §4 补「合并发版」例外条款（明确确认 / 分节登记 / 逐项说明 / 不夹带第二个新能力）
- [ ] `bash scripts/check-version.sh` 与 `bash scripts/package-release.sh`：产出 `dist/KeyPulse-1.1.0/`、zip、直下 exe 与 `SHA256SUMS.txt`
- [ ] 全量验证：`go test ./...`、页面回归 9 项、`--version`、PE subsystem、校验和全项 OK
- [ ] 中文分功能点提交（心跳修复 / 升版变更）、`v1.1.0` annotated tag、推送 main 与 tag（只用 `git -c` 单次传参，不改凭据配置）
- [ ] 创建 latest Release：正文含两个主题、校验、隐私边界、Defender 与已知限制；上传 exe / zip / SHA256SUMS 三个资产
- [ ] 回读三个资产字节逐一比对 SHA256、确认 latest 指向；本机根目录 exe 替换为 1.1.0 正式版
- [ ] 回填台账、工作日志与项目长期笔记；终态复核 git 配置与工作区

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
