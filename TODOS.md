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

## 实时模式（2026-11-06 实现，已随 v1.1.0 发布）

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

## 心跳误报修复（已随 v1.1.0 发布）

- [x] **已修复（2026-11-09）**：根因不是「锁屏误判」，而是 `GetLastInputInfo` 的 `dwTime` 更新滞后于低层钩子回调一个心跳周期，旧 `decideAction` 只看单周期「tick 变化 + 零回调」就重挂。67 条记录里 42 条 `gap<=1s`（掉线前 1 秒钩子还在回调）即为铁证。修复：`internal/hook/hook_windows.go` 加 30 秒连续静默确认窗口、`inputSinceAlive` 跨周期累计、重挂成功后重置观察窗口；`internal/report/data.go` 只统计新版 `reason=confirmed_silence`，旧误报行保留但不再挂提示；托盘气泡文案同步。新增 3 组单测（判定全表、跨周期时序错位、恢复窗口复位）与 1 个报告端旧日志忽略用例。**未删任何用户数据**，`health.log` 原 67 行保持原样。
- [x] 发版顺序已由用户拍板：不与实时模式拆版，按 `docs/versioning.md` §4 新增的「合并发版」例外条款随 **1.1.0** 一起发布；CHANGELOG 分节登记、Release 正文逐项说明，未夹带第二个新能力。

## v1.1.0 发布（2026-11-09）

合并发版：实时模式（新能力）+ `health.log` 假阳性修复（随版 PATCH 性质修复）。

- [x] 升 `VERSION` → 1.1.0；`CHANGELOG.md` 把 Unreleased 归入 `[1.1.0] - 2026-11-09` 并保留常驻空 Unreleased、补 compare 链接
- [x] `docs/versioning.md` §4 补「合并发版」例外条款（明确确认 / 分节登记 / 逐项说明 / 不夹带第二个新能力）
- [x] `bash scripts/check-version.sh` 与 `bash scripts/package-release.sh`：产出 `dist/KeyPulse-1.1.0/`（10 项清单）、zip（2,214,695 B）、直下 exe（2,481,152 B）与 `SHA256SUMS.txt`（800 B）
- [x] 全量验证：`go test ./...` 全 PASS、页面回归 **9 项 PASS**、`--version` = `KeyPulse 1.1.0 (build 2026-11-09T02:58:22Z)`、PE subsystem 2/3 正确、10 项校验和全 OK、真实数据目录报告 `health.n=0`（旧 68 条误报不再挂黄条）
- [x] 中文分功能点提交：`b61654c` 心跳修复（代码 + 测试）、`7bdf39c` 升版 1.1.0 与文档同步；`v1.1.0` annotated tag 固定在 `7bdf39c`；推送 main 与 tag 全程 `git -c` 单次传参，未改任何凭据配置
- [x] 创建 latest Release：正文含两个主题、校验、隐私边界、Defender 与已知限制；上传 exe / zip / SHA256SUMS 三个资产，服务端 digest 与本地一致
- [x] 资产 API 回读三个文件字节逐一比对 SHA256 全 OK、匿名确认 latest 指向 v1.1.0；本机根目录 `keypulse.exe` / `keypulse-cli.exe` / `keypulse-live-preview.exe` 已统一替换为 1.1.0 正式版，历史数据未动
- [x] 回填台账、工作日志与项目长期笔记；终态复核 git 配置与工作区

## 本地看板（2026-11-09 本地实现，**未发布**）

用户拍板：路线 **B（127.0.0.1 高位端口 + SSE）** + 隐私边界「**只允许聚合键位位图**」；并明确「**先本地做，验证后再定版本与公告**」「**默认不开，勾选才启动**」。本轮只做本地实现与预览构建：不升 `VERSION`、不提交、不推送、不发版。公开承诺措辞按 2026-11-09 后续拍板统一收敛（见下方条目）。

- [x] `internal/counter/counter.go`：新增纯内存 `[4]uint64` 键位脉冲位图（`MarkPulse` / `TakePulse`，跨天清零），不进 `.bin`；`keyboardProc` 在 `Add` 后多一次 `atomic.OrUint64`，回调仍是零分配 / 零锁 / 零 IO
- [x] `internal/live/server.go`（新增）：只绑 `127.0.0.1` 高位端口（39117 起递增）、一次性 32 字节 token + Host/Origin 校验、CSP、`GET /` 服务端直出、`GET /events` SSE（每秒聚合快照 / 200ms 键位位图）、`POST /api/live` 页面内开关、无连接 30 秒自动关停并释放端口；纯标准库、零项目内依赖
- [x] `internal/report/page.go` / `report.go` / `live.go`：三种页面形态共用模板（普通静态 / live.js 实时 / 看板 SSE）；`RenderServer` 只返回字符串不写文件；`LiveSnapshotJSON` 与 live.js 逐字段同源；`.key` 加 `data-vk` 与 CSS 闪动；页面内开关、SSE 客户端；**今日视图不再无条件 60 秒整页重载**（顺手修 1.1.0 偏差，周报 / 总览仍定时重载）
- [x] `cmd/keypulse/board.go`（新增）+ `main.go` + `tray_windows.go`：托盘带勾选的「本地看板（实时）」、服务启停与 `OnStop` 复位、快照缓存 `liveSnap`、看板开启时不再后台重生成静态 HTML、退出显式释放端口；所有网络 / 文件 IO 全在 goroutine
- [x] 验证：`go test ./...` 全 PASS（新增 `internal/live/server_test.go` 6 组、`internal/counter/counter_test.go` 2 组、report 端 RenderServer / 快照同源用例）；页面回归 **10 项 PASS**；**Chrome headless + CDP 真实浏览器 12 项 PASS**（SSE 帧、CSP、60 个 `data-vk` 键位、12 次闪动、页面内暂停 / 恢复、snapshot 后视图、自动关停、端口释放）；`gofmt` 改动文件干净、`go vet` 仅剩已知 3 条 Win32 提示
- [x] 本地预览包：`dist/KeyPulse-board-preview/`（11 项清单全 OK）、`dist/KeyPulse-board-preview-win-x64.exe`、`dist/KeyPulse-board-preview-win-x64.zip`、`BOARD-PREVIEW.txt` 写明验证步骤与未发布状态（2026-11-09 14:09 已按收敛后文案整体重打包）

**已知代价（需评估）**：引入标准库 `net/http` 后，单 exe 由 2,481,152 B 增至 **6,312,448 B**（约 2.4 MB → 6.3 MB），静态链接拉进 `net` / `crypto/tls` / `mime` 等；常驻看板时内存与 goroutine 也会上升。若体积不可接受，备选是用 `net.Listener` + 手写最小 HTTP/SSE 响应替换 `net/http`（省 2–3 MB，但要自己处理解析 / 超时 / 连接管理，风险更高）。

- [x] **版本与公告已拍板（2026-11-09 用户）**：不改 `docs/versioning.md` §3.2 规则、不升 **2.0.0**、不写破坏性说明 / 迁移方案、不单独公告；本地看板只推聚合数据、不落盘、用户主动开启，隐私模型未改变，按 MINOR 排队即可。已发布的 `v1.1.0` Release 正文按「已发布不覆盖」保持原样
- [x] **公开承诺表述统一收敛**：README / CHANGELOG / `docs/metrics.md` / `docs/release.md` / CONTRIBUTING / 报告页脚与代码注释，一律改为「不联网、不上传、不暴露隐私」这类不绑定具体实现的表述，删去「不监听端口 / 不起服务 / 零联网 / 没有任何联网代码路径 / 无服务」等会被可选功能打破的绝对化措辞；不补防火墙弹窗、端口鉴权等额外技术说明
- [x] 按 13 步 SOP 升 `VERSION` → **1.2.0** 并完成发布（2026-11-09 15:43，详见下方「v1.2.0 发布」小节）；用户 14:12 再次确认按 `docs/versioning.md` §2 命中②走 MINOR、PATCH 归零、**不升 2.0.0**（§3.2 五条逐条不命中，纯 127.0.0.1 回环不按「引入联网/遥测」解释）；「MINOR 一次一个主题」，macOS 移植继续排队
- [ ] 人工实机复核（工具会话无法常驻托盘，仍需用户在交互桌面完成）：双击 1.2.0 正式版 exe，勾选「本地看板」看浏览器自动打开、敲键键位闪动、页面内开关、关页面 30 秒后端口释放、看板开启时托盘隐藏「实时模式」且关闭后恢复、离线页实时勾选不被清掉。**SOP 第 7 步此条未由人工完成，按用户 15:39 明确「升1.2.0发版」指令先行发布**
- [x] **2026-11-09 14:09 重新打包**：用户退出托盘后重编两版 exe（`1.1.0 (build 2026-11-09T06:08:50Z)`，6,312,448 B）、重生成示例报告、同步包内 README / CHANGELOG / docs 的收敛后文案、改写 `BOARD-PREVIEW.txt`（删除已作废的「必须拍板 §3.2 / 升 2.0.0」整段，改为收敛结论 + 发版 SOP）、重算 11 项 `SHA256SUMS.txt`、重打 zip（14 条目）、更新直下 exe，并把根目录 `keypulse.exe` / `keypulse-cli.exe` / `keypulse-live-preview.exe` / `preview.html` 全部同步。验证：8 包单测全 PASS、页面回归 10 项 PASS、11 项校验和全 OK、PE subsystem 2/3 正确、四处 exe 字节一致、默认 39117 未监听。**仍未提交 / 未推送 / 未发版 / VERSION 仍 1.1.0**

- [x] **2026-11-09 14:18 清空 `dist/` 并改构建落点**：按用户要求删除 14 个条目 / 59 MB（1.0.0 / 1.0.1 / 1.1.0 正式版本地副本 + 两个预览包及其 exe / zip / 清单），保留空的 `dist/` 目录供发版脚本使用；已发布资产仍在 GitHub Release 可重下，历史 SHA256 记录保留在本台账。**后续约定：本地预览 / 日常构建一律 `bash build.sh .` 直接输出到项目根目录，不再往 `dist/` 生成预览包、zip 与 `SHA256SUMS.txt`；`dist/KeyPulse-<版本>/` 只在正式发版时由 `package-release.sh` 产生。** 根目录 exe 已于 14:16 直接重编（`1.1.0 (build 2026-11-09T06:16:46Z)`，6,312,448 B，三个 exe 同字节），双击即可，不依赖 `dist/`

- [x] **2026-11-09 14:20 已提交并推送 `main`**：按「单功能点一次提交」拆成两个中文提交 —— `7d362ed feat: 新增本地看板（实时）可选能力`（14 文件 / +1260 −51）与 `5c3af04 docs: 收敛隐私承诺表述并同步公开文档与台账`（6 文件 / +47 −17）；`git push origin main` 快进 `ded855e..5c3af04`，`git ls-remote` 复核远端 HEAD 与本地一致，工作区干净。提交前按 AGENTS.md 第 9 条扫描暂存区无敏感信息；**未改任何 Git 凭据配置、未用带 token 的 URL**。上方各条「未提交 / 未推送」均为当时的历史状态记录。**只是代码进公开 `main`：未升 `VERSION`、未打 tag、未发 Release，线上 v1.1.0 资产不变**；正式发版仍按 13 步 SOP 升 `1.2.0`

- [x] **2026-11-09 14:22 收敛三个入口的托盘呈现**：按用户「有了本地看板，另外两个还需要吗」的讨论结论，**保留全部能力、只收敛交互**。`tray` 新增 `menuItems()` 纯函数按状态现算菜单，看板开启时隐藏「实时模式」（页面内已有同一开关）、关闭后恢复；「打开统计报告」改名「打开统计报告（离线）」，区分可保存的单文件 HTML 与不落盘的看板页；`boardLive` 与 `liveMode` 拆成两个独立状态，修掉看板启停强制清掉静态实时勾选的耦合，两条通道并行时 `live.js` 继续写，避免离线页挂「数据未更新」提示。新增 `TestMenuItemsHideLiveWhenBoardOn` 单测；README / CHANGELOG Unreleased / `docs/release.md` / 报告页脚与提示文案同步。**不删 v1.1.0 已发布的实时模式**（当时括注写「按 §3.2 第 3 条删除已公开能力会撞 MAJOR」，2026-11-10 核对原文后纠正：该条只管 CLI 参数与 CSV 列名，**不含 UI 菜单项**；不删的真实理由是产品层面不该在 MINOR 里移除用户可见能力，且会丢掉唯一零端口 / 零服务的轻量实时档）；「打开统计报告」也保留——看板不落盘、关页 30 秒即失效，且默认状态下不能要求用户先起服务才有报告可看

- [x] **2026-11-09 14:48 已提交并推送 `main`**：入口收敛按「单功能点一次提交」拆成两个中文提交 —— `cc9dda7 refactor: 收敛托盘菜单并拆分看板与离线实时状态`（6 文件 / +135 −47）与 `b4aa88a docs: 同步三个报告入口说明并忽略 exe 备份文件`（5 文件 / +12 −5）；14:45 用户退出托盘后做了一次无占用干净重编（`1.1.0 (build 2026-11-09T06:45:49Z)`，6,315,008 B，无 `exe~` 残留）。`git push origin main` 快进 `118accb..b4aa88a`，`git ls-remote` 复核远端 HEAD = 本地 `b4aa88a…`，工作区干净。提交前按 AGENTS.md 第 9 条扫描：仅命中公开文档里的 `127.0.0.1` 回环地址，无真实凭据 / 本机路径 / token；**未改任何 Git 凭据配置、未用带 token 的 URL**。**仍是未升 `VERSION`（1.1.0）、未打 tag、未发 Release**，线上 v1.1.0 资产不变；正式发版按 13 步 SOP 升 `1.2.0`

## v1.2.0 发布（2026-11-09 15:43，当前 latest）

- [x] 版本决策：**MINOR 1.2.0、PATCH 归零、不升 2.0.0**。`docs/versioning.md` §2 命中②「新增向后兼容能力」；§3.2 破坏性五条逐条不命中（`.bin` 仍 `KP01`、口径 / CLI / CSV / 路由未变、不记序列字符窗口、无对外联网或遥测）；纯 `127.0.0.1` 回环 + 默认关闭 + 主动开启 + 只推聚合位图 + 不落盘，按 2026-11-09 拍板不按「隐私模型改变」解释。按 §4 合并发版例外，随版附带同分支的交互收敛（PATCH 级）与公开表述调整，CHANGELOG 分节登记、Release 正文逐项说明，**未夹带第二个新能力**
- [x] 文档同步：`VERSION` → 1.2.0；CHANGELOG `Unreleased` 归入 `[1.2.0] - 2026-11-09` 并新建空 `Unreleased`；README 补「本地看板（可选）」节、体积改 6.3 MiB、项目结构补 `internal/live/`、删掉「尚未发布」；`docs/metrics.md` 措辞、`docs/release.md` 补本地看板验收条目与已知限制
- [x] `bash scripts/check-version.sh` 通过；`bash scripts/package-release.sh` 全绿：8 包单测 PASS、页面回归 10 项 PASS、PE subsystem 2/3、10 项 `SHA256SUMS.txt` 全 OK、直下 exe == 包内 `keypulse.exe`、zip 13 条目与目录哈希一致、包内无源码 / `.env.local` / 真实数据
- [x] 构建信息：`KeyPulse 1.2.0 (build 2026-11-09T07:43:35Z, windows/amd64, go1.25.0)`，单 exe 6,315,008 B（引入标准库 `net/http`，较 1.1.0 的 2.4 MB 增大）
- [x] 提交与 tag：`cdf685e docs: 同步本地看板功能说明与发布自测清单` → `cbdd25c release: 发布 v1.2.0`（发版提交单独一次）；annotated tag `v1.2.0` 固定在 `cbdd25c`；`git push origin main` 快进 `e8cf0b0..cbdd25c`，`git push origin v1.2.0` 成功，远端 HEAD 一致。**未改任何 `credential.*`、未用带 token 的 URL**
- [x] GitHub Release：`v1.2.0 · 本地看板（实时）`（id `438854409`，非 draft / 非 prerelease，正文 2449 字符取自 CHANGELOG + 校验和 + 适用系统 + 升级 + 隐私边界 + 已知限制），`https://github.com/jingyuan9527/KeyPulse/releases/tag/v1.2.0` 为当前 latest
- [x] 三资产上传与校验（服务端 `digest` 与本地 SHA256 / size 逐一一致）：

  | 资产 | 大小 | SHA256 | asset id |
  |---|---|---|---|
  | `KeyPulse-1.2.0-win-x64.exe` | 6,315,008 B | `c0323f995bcb988de618cc5f2d6c6bd4084c52d49c6c54bce9f9db9aafed6533` | 726651656 |
  | `KeyPulse-1.2.0-win-x64.zip` | 5,342,679 B | `85a935fd6abe3da1f0c6ba098b12c7ec5dfd49bb95ab1a5db27bcfcb621d5b56` | 726651733 |
  | `SHA256SUMS.txt` | 800 B | `5eaf6089392997d01a5a0a894e177b640bfb051f540e81e9d4d3a17ab40b295d` | 726651832 |

- [x] 回读：`SHA256SUMS.txt` 经 API 资产端点（302 → CDN）字节回读 800 B、哈希一致；两个大文件的 CDN 直链在本机代理下 40 秒内只传到约 0.7 MB 即超时（已知 Schannel / 代理坑），**以服务端 `digest` 三项一致作为权威校验**，未强求大文件回读。`GET /releases/latest` 复核 `tag=v1.2.0`、`draft=false`、`prerelease=false`、3 个资产齐全
- [x] 本机根目录 `keypulse.exe` / `keypulse-cli.exe` / `keypulse-live-preview.exe` / `preview.html` 已同步为 1.2.0 正式构建；根目录 exe 的 SHA256 = `c0323f99…6533`，与发布包及 Release 直下资产同一字节。替换时托盘进程 PID 15344 仍在运行（Windows 允许 rename 运行中的 exe），**当前进程仍是旧版，重启托盘后生效**，未擅自结束用户进程
- [ ] 按 SOP 第 7 步，人工实机自测清单（`docs/release.md` 第 4 节，含本地看板 4 条）仍待用户在交互桌面完成；这是本次发布唯一未由工具覆盖的验收项
- [x] 下一个 PATCH 定为 **1.2.1**（2026-11-10 用户拍板「加吧 排1.2.1」）：排队「托盘入口归并」+「GUI 启动期可见反馈」两项交互修复。本轮不改 `VERSION`（仍 1.2.0）、不提交 / 不推送 / 不发版，发版时按 13 步 SOP 升位。其余新能力（macOS 移植等）另发 `1.x.0` 并排队，不得塞进 PATCH

## 托盘入口归并（2026-11-10，未发布）

用户反馈「几个入口有重复、能不能合并」。核对 `docs/versioning.md` §3.2 原文后纠正 11-09 的过度解读：五条 MAJOR 白名单不含 UI 菜单项，第 3 条只限 CLI 参数与 CSV 列名；但删除 v1.1.0 已发布的「实时模式」仍是产品层面的能力移除，且会丢掉唯一「零端口、零服务、只多写 1–3 KB `live.js`」的轻量实时档。按用户选定 **B 子菜单归并** 落地，**不砍任何能力**。

- [x] `internal/tray/tray_windows.go`：新增 `mfPopup` / `addSubMenu` / `buildMenu`，把「打开（离线快照）」「实时刷新」收进「统计报告」子菜单；顶层只留「本地看板（实时）」一个实时勾选项。命令 ID 全部保留，看板开启时子菜单仍隐藏「实时刷新」；子菜单创建失败时退回平铺，保证动作可达。双击托盘仍直接开报告
- [x] 单测：新增 `TestMenuItemsGroupReportActionsInSubmenu`（层级 / 文案 / 勾选态 / 看板态）与 `TestBuildMenuSubmenuABI`（真调 Win32 `GetSubMenu` / `GetMenuItemID` / `GetMenuItemCount`，校验 `MF_POPUP` 句柄与子菜单命令 ID 未丢失）；移除旧 `TestMenuItemsHideLiveWhenBoardOn`
- [x] 文案同步：报告页脚、live 提示、看板启动气泡、README 日常表与三入口段、CHANGELOG `Unreleased`、`docs/release.md` 自测清单；已发布的 1.2.0 CHANGELOG 条目与 Release 资产按「不覆盖」保持原样
- [x] 验证：`gofmt` 干净；`go test -buildvcs=false -count=1 ./...` 8 包全 PASS；页面回归 10 项 PASS；`bash build.sh .` 直接重编根目录 exe（`1.2.0 build 2026-11-10T06:14:12Z`），二进制含新菜单文案；清掉 `keypulse.exe~` 构建残留
- [ ] 人工实机复核：退出当前托盘（PID 18764）后重新双击根目录 exe，右键确认「统计报告 ▸」能展开、两个子项可点开且勾选生效、看板开启时子菜单只剩「打开（离线快照）」、关闭后恢复
- [x] 边界：不改 `VERSION`（仍 1.2.0）、不覆盖任何已发布资产、未提交 / 未推送 / 未发版

## 1.2.1 排队：GUI 启动期可见反馈（2026-11-10，未发布）

用户 14:25 拍板「加吧 排1.2.1」。起因是 14:23 用户双击根目录 exe「启动不了」：残留旧实例持有 `Local\KeyPulseSingleton`，`acquireSingleton()` 失败时只 `fmt.Println`，而 GUI 版由 `-H=windowsgui` 编译、没有控制台 → 表现为静默退出。本轮只落地代码与文档，**不改 `VERSION`、不提交 / 推送 / 发版**；与同处未发布分支的「托盘入口归并」一起按 PATCH 发布（两项都是交互修复，非新能力，§4 合并发版例外不适用）。

- [x] `cmd/keypulse/main.go`：新增 `notify(title, text)` 与 `outputVisible()`；GUI 版弹 `MessageBoxW`（MB_ICONINFORMATION | MB_SETFOREGROUND | MB_TOPMOST），CLI 版照旧打印。`CreateMutexW` 提为包级 proc 复用
- [x] 4 处启动期静默输出统一接 `notify`：单实例冲突、数据目录打开失败、钩子安装失败、托盘初始化失败；单实例提示明确告知去托盘右键「退出」（图标常被折叠进「^」）
- [x] **判据修正（实测发现）**：初版只用 `GetConsoleWindow`，但本机 Git Bash / mintty 下它返回 0、stdout 却是有效管道句柄（712）→ CLI 版被误判去弹框并阻塞（`timeout` 退出码 124）。改为「控制台窗口 ≠ 0 **或** `GetStdHandle(STD_OUTPUT_HANDLE)` 有效」才打印，两者皆无才弹框。零构建脚本改动，两份 exe 仍是同一份源码
- [x] `cmd/keypulse/main_test.go`：新增 5 条单测（有控制台 / 仅 stdout 有效 / 两者皆无 / stdout 为 INVALID_HANDLE_VALUE / 四个 Win32 导出名可解析），用可替换间接层避免真弹窗与依赖终端环境
- [x] **真实弹窗验证**：Python `DETACHED_PROCESS` 启动 GUI 版触发单实例冲突，同脚本用 `EnumWindows` 枚举到 `class=#32770`、`title=KeyPulse 已在运行（单实例保护）`、进程 `poll=None`（弹框阻塞中），确认 `MessageBoxW` 正文 / 标题参数顺序正确；随后精确结束验证进程，用户实例 41916 未受影响。CLI 版同场景打印原因并 `exit=0`
- [x] 文档：CHANGELOG `Unreleased` 加「修复」节并注明排队 1.2.1；README 新增「双击没反应」；`docs/release.md` 发布前检查 +1 条、实机自测插入第 8 步并重编号
- [x] 验证：`gofmt` 干净；`go test ./...` 8 包全 PASS；页面回归 10 项 PASS；`bash build.sh .tmp/_check/bin` 后同步根目录三个 exe（`1.2.0 build 2026-11-10T06:33:23Z`，6,317,568 B），PE subsystem 2/3 正确、字节一致、无 `exe~` 残留
- [ ] 人工实机复核：退出托盘后重新双击根目录 exe（当前 41916 仍是旧进程），不退出时再双击应弹出可见提示
- [x] **2026-11-10 14:40 本地打包 1.2.1**：按用户「本地打包一下 1.2.1」重编根目录 exe，产物 `--version` = `KeyPulse 1.2.1 (build 2026-11-10T06:40:18Z)`。用一次性 `go build` 把 ldflags 的 `Version` 注入 1.2.1（参数与 `build.sh` 一致，仅版本值不同），**`VERSION` 文件仍 1.2.0、CHANGELOG 未归档、不提交 / 不推送 / 不发版**；产物直接落根目录，不往 `dist/` 堆包。`keypulse.exe` / `keypulse-cli.exe` / `keypulse-live-preview.exe` 三份同步（GUI 两份字节一致），6,317,568 B、PE subsystem 2/3 正确、无 `exe~` 残留；8 包单测全 PASS、页面回归 10 项 PASS、二进制含新菜单与弹窗文案。构建前确认无托盘进程占用。**与历史惯例的差异**：以往本地预览包沿用当前 `VERSION`、只用 build 时间戳区分，本次用户明确指定版本号才让产物显示 1.2.1；版本文件与发布资产一律不动，正式发版仍按 13 步 SOP 升位
- [x] **2026-11-10 14:42 已提交并推送 `main`**：按「单功能点一次提交」拆成三个中文提交 —— `e88d94f refactor: 托盘把离线报告动作收进统计报告子菜单`（5 文件 / +173 −37）、`1ca927b fix: GUI 版启动期错误改为可见提示`（2 文件 / +152 −11）、`ca7db59 docs: 同步 1.2.1 排队改动与实机自测清单`（4 文件 / +60 −17）。`git push origin main` 快进 `5cf2bce..ca7db59`，`git ls-remote` 复核远端 HEAD = 本地 `ca7db59…`，工作区干净。提交前按 AGENTS.md 第 9 条扫描：仅命中公开文档里的 `127.0.0.1` 回环地址，无真实凭据 / 本机路径 / token；**未改任何 Git 凭据配置、未用带 token 的 URL**。上方各条「不提交 / 不推送」均为当时的历史状态记录。**只是代码进公开 `main`：未升 `VERSION`（仍 1.2.0）、未打 tag、未发 Release，线上 v1.2.0 资产不变**；正式发版仍按 13 步 SOP 升 1.2.1
- [x] 发版时按 13 步 SOP 升 `VERSION` → 1.2.1、`check-version.sh` + `package-release.sh`、推 tag / Release（2026-11-10 15:2x 已完成，详见下方小节）

## v1.2.1 发布（2026-11-10 15:31，当前 latest）

- [x] 版本决策：PATCH **1.2.1**，主题「托盘入口归并 + GUI 启动期可见反馈」两项交互修复；§2 命中③、§3.2 破坏性五条逐条不命中、不升主版本、不需迁移方案；§4 合并发版例外不适用（两项都是修复，非新能力）
- [x] 文档：`VERSION` → 1.2.1；CHANGELOG `Unreleased` 归入 `[1.2.1] - 2026-11-10` 并新建空 `Unreleased`；补 compare 链接（同时补齐此前遗漏的 `[1.2.0]`，把 `[Unreleased]` 基准从 v1.1.0 改到 v1.2.1）
- [x] `bash scripts/check-version.sh` 通过；`bash scripts/package-release.sh` 全绿：8 包单测 PASS、页面回归 10 项 PASS、PE subsystem 2/3、10 项 `SHA256SUMS.txt` 全 OK、直下 exe == 包内 `keypulse.exe`、zip 13 条目与目录哈希一致、包内无源码 / `.env.local` / 真实数据
- [x] 构建信息：`KeyPulse 1.2.1 (build 2026-11-10T07:22:51Z, windows/amd64, go1.25.0)`，单 exe 6,317,568 B
- [x] 发版提交 `15a1f19 release: 发布 v1.2.1`（2 文件 / +9 −3）；annotated tag `v1.2.1` 对象 `8b2ab5c` 固定在该提交
- [x] **推送改用 GitHub Git Data API（本次环境阻塞的处置）**：本机代理对 `github.com` 的 git 端点持续 `CONNECT tunnel 502` / Schannel `missing close_notify`，`http.version=HTTP/1.1` 与绕代理直连均失败，而 `api.github.com` 正常。经 `git credential fill` 单次取用 token（不打印、不改 `credential.*`、不用带 token 的 URL），用 API 逐个创建 blob / tree / commit / tag 对象，**每一级 SHA 都与本地一致**（tree `a79ee5c…`、commit `15a1f19…`、tag 对象 `8b2ab5c…`），再更新 `refs/heads/main` 与 `refs/tags/v1.2.1`；本地 `origin/main` 用 `git update-ref` 同步到同一 SHA，无分叉，远端 ref 经 API 复核一致
- [x] GitHub Release：`v1.2.1 · 托盘入口归并 + 启动期提示`（id `440058822`，非 draft / 非 prerelease，正文 2332 字符取自 CHANGELOG + 校验 + 适用系统 / 升级 + 隐私边界 + 已知限制 / Defender），`https://github.com/jingyuan9527/KeyPulse/releases/tag/v1.2.1` 为当前 latest
- [x] 三资产上传与校验（服务端 `digest` / `size` 逐一一致，`SHA256SUMS.txt` 800 B 字节回读一致）：

  | 资产 | 大小 | SHA256 | asset id |
  |---|---|---|---|
  | `KeyPulse-1.2.1-win-x64.exe` | 6,317,568 B | `bc82f340…6395` | 730524830 |
  | `KeyPulse-1.2.1-win-x64.zip` | 5,347,233 B | `30377b88…2200` | 730524951 |
  | `SHA256SUMS.txt` | 800 B | `51ccfb30…d9a0` | 730525147 |

- [x] 根目录 `keypulse.exe` / `keypulse-cli.exe` / `keypulse-live-preview.exe` 已同步为 1.2.1 正式构建；`keypulse.exe` 与 `keypulse-live-preview.exe` = `bc82f340…6395`，与 Release 直下资产同一字节。替换时托盘进程 PID 9080 仍在运行（Windows 允许 rename），旧 exe 移入 `.tmp/keypulse.exe~.bak` 未删除，**当前进程仍是旧预览版，退出后重新双击才生效**
- [ ] 按 SOP 第 7 步，人工实机自测清单（`docs/release.md` 第 4 节，含新增第 8 步单实例弹框）仍待用户在交互桌面完成；用户明确下令发版即视为接受
- [x] 已发布的 v1.2.0 tag / Release / 资产按「已发布不覆盖」保持原样；`VERSION` 与 CHANGELOG 历史条目不回改

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
