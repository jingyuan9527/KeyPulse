# KeyPulse 发布说明与自测清单

本文件是**通用发布流程**，不绑定具体版本号。版本号只写在 `VERSION` 与 `CHANGELOG.md`，升位规则与发版 SOP 见 `docs/versioning.md`。

## 1. 交付物

| 文件 | 用途 |
|---|---|
| `keypulse.exe` | 日常使用。无控制台，托盘常驻，全局统计输入次数 |
| `keypulse-cli.exe` | 调试与自检。保留控制台，支持 `--version` / `--selftest` / `--report-only` |
| `example-report.html` | 35 天模拟数据生成的离线报告，不启动程序即可预览三个视图 |
| `README.md` / `CHANGELOG.md` / `LICENSE` | 使用说明、逐版本变更、许可证 |
| `docs/metrics.md` / `docs/versioning.md` / `docs/release.md` | 统计口径与数据格式、版本规则、本文件 |
| `SHA256SUMS.txt` | 包内全部文件的校验和（不含自身） |

发布包同时提供目录形式 `dist/KeyPulse-<版本>/`、压缩包 `dist/KeyPulse-<版本>-win-x64.zip`，以及免解压直下的 `dist/KeyPulse-<版本>-win-x64.exe`（与包内 `keypulse.exe` 是同一份字节）。后两者连同 `SHA256SUMS.txt` 一起上传 GitHub Release。

**免安装、单文件、零外部依赖**：不需要 .NET / VC++ 运行库 / WebView2 / SQLite，不写注册表启动项、不装驱动、不注入进程。数据只写在 `%LOCALAPPDATA%\KeyPulse\`。

示例报告由 `--selftest` 生成，页脚的「数据目录」是自检临时路径，仅用于演示界面，不是真实采集数据。

运行环境：Windows 10 / 11，x64。构建：Go 1.25.0，`CGO_ENABLED=0`。

## 2. 从源码复现构建

```bash
bash build.sh                        # 输出到项目根目录
bash build.sh dist/KeyPulse-<版本>   # 输出到指定目录
bash scripts/check-version.sh        # 发版前版本一致性校验
bash scripts/package-release.sh      # 完整发布包（测试 + 构建 + 文档 + 校验和 + zip + 直下 exe + 验收）
```

版本号唯一来源是根目录 `VERSION` 文件，`build.sh` 用 `-ldflags -X` 注入 `internal/version`。报告页脚与托盘提示都显示同一份构建信息，用户反馈问题时可直接报 `--version` 输出。

构建脚本默认使用 `PATH` 里的 `go`；便携版 Go 可复制 `.env.local.example` 为 `.env.local` 指定 `KP_GO_BIN` / `KP_GOROOT` / `KP_GOCACHE`，该文件不入库、不随发布包分发，因此不同机器上构建产物不含个人路径。

## 3. 发布前检查清单

- [ ] `bash scripts/check-version.sh` 通过（VERSION 格式、CHANGELOG 条目、tag 未重复、文档无写死版本号）
- [ ] `go test -buildvcs=false -count=1 ./...` 全 PASS
- [ ] `node scripts/check-page.js .tmp/_selftest/report.html` 单日 / 周报 / 总览等 10 项全 PASS
- [ ] 两个 exe 的 `--version` 一致，且与 `VERSION` 文件一致
- [ ] `keypulse.exe` PE subsystem = 2（无控制台），`keypulse-cli.exe` = 3（控制台）
- [ ] `SHA256SUMS.txt` 与实际文件一致。Git Bash 下 `sha256sum -c SHA256SUMS.txt`；PowerShell 下 `Get-FileHash keypulse.exe -Algorithm SHA256` 逐项比对
- [ ] 直下资产 `KeyPulse-<版本>-win-x64.exe` 与包内 `keypulse.exe` 字节一致，其 SHA256 等于清单中 `keypulse.exe` 一行
- [ ] `KeyPulse-<版本>-win-x64.zip` 能正常解压，包内文件与目录形式一致
- [ ] 发布包内不含源码、`.tmp/`、`.workbuddy/`、`.env.local`、任何真实数据
- [ ] README / metrics / release / CHANGELOG 四份文档的功能描述与已知限制一致
- [ ] GUI 版重复双击（已有实例在跑）时弹出可见提示，而不是静默退出；CLI 版仍打印到控制台

## 4. 用户侧实机自测清单

按顺序做一遍，约 5 分钟：

1. `keypulse-cli.exe --version` → 应输出 `KeyPulse <版本> (build ...)`，版本号与 `VERSION` 一致。
2. `keypulse-cli.exe --selftest` → 生成 35 天模拟数据与报告，**不挂钩子、不采集真实输入**；浏览器打开可见单日 / 周报 / 总览三个视图。
3. 双击 `keypulse.exe` → 右下角出现蓝底键帽 + 绿点图标，悬停提示 `KeyPulse v<版本> · 采集中`。
4. 敲约 20 下键盘、点 5 下鼠标 → 双击托盘打开报告，按键与点击数对得上。
5. 右键 → **暂停采集** → 图标转灰 + 琥珀点、提示变「已暂停」；再点恢复。
6. 页面开着继续输入，60 秒后数字自动上涨（仅托盘「统计报告 → 打开（离线快照）」后的 10 分钟保活窗口内）。
7. 右键 → **导出 CSV** → 用 Excel 打开，中文表头与数据正常。
8. **不退出托盘**，再次双击 `keypulse.exe` → 弹出「KeyPulse 已在运行（单实例保护）」提示框；点确定后已在运行的实例不受影响。用 `keypulse-cli.exe` 做同一件事应改为在控制台打印原因。
9. 右键 → **退出** → 进程消失；`%LOCALAPPDATA%\KeyPulse\` 里的 `.bin` 仍在。
10. 再次启动 → 当天数字接着累加，不清零。
11. 正常操作（含移动鼠标、锁屏解锁、切换窗口）后，目录下不应新增 `health.log`；只有新增 `reason=confirmed_silence` 才代表确认过采集中断，报告顶部会标注缺口。旧版遗留的 `reason=input_without_callback` 行保留但不再触发提示。

**实时模式额外验收**（仅当该版本包含此开关）：

- 右键 → **统计报告 → 实时刷新** → 子菜单项出现勾选、弹出静默气泡，数据目录多出 `live.js`；`live.js` 只含当天聚合计数，不含任何按键序列或文本。
- 打开（离线快照）后继续敲键，今日卡片数字每秒跳动，页面**不整页重载**：不闪烁、滚动位置不丢、正在选的日期不被打断。
- 切到周报 / 总览不报错，仍按 60 秒刷新；关闭开关后 `live.js` 被删除、页面回落为普通刷新，重启程序开关默认关闭。

**本地看板额外验收**（仅当该版本包含此开关）：

- 默认启动不勾选，行为与旧版一致；右键 → **本地看板（实时）** → 浏览器自动打开实时统计页。
- 敲键时键盘热力图对应键位闪动，页面上方开关可暂停 / 恢复推送，今日数字每秒更新。
- 看板开启时「统计报告」子菜单不再显示「实时刷新」；关闭看板后该项恢复，离线页原本勾选的实时模式状态不受影响。
- 关闭全部看板页面 30 秒后服务自动停止；退出程序后不再占用回环地址。

## 5. Defender 误报处理

全局低层键盘钩子 + 本地落盘，在杀软看来与键盘记录器特征重合，**首次运行可能被 Windows Defender 拦截**。这是预期行为，不要用加壳/混淆去绕，那只会坐实恶意判定。

本工具主动砍掉了对应的能力：只记 VK 计数、不记字符文本/按键序列/窗口标题/剪贴板、不截图、不联网、不上传、不暴露隐私、不隐藏进程、不写启动项、代码可自查。若被拦截，手动加排除项：

1. Windows 安全中心 → 病毒和威胁防护 → 「管理设置」。
2. 排除项 → 添加或删除排除项 → 添加**文件**，选中 `keypulse.exe`。
3. 如数据目录写入被拦，再添加**文件夹** `%LOCALAPPDATA%\KeyPulse`。
4. 若提示「受控文件夹访问」阻止，在同一页的「允许应用通过受控文件夹访问」里加入该 exe。
5. 不要整体关闭实时防护；排除范围只给这一个文件。

想自行核验可把 exe 传到 VirusTotal 查看检出项，或对照源码检查 `internal/hook/` 与 `internal/store/` 两个目录。

## 6. 统计口径与数据文件

- 一天一个 `YYYYMMDD.bin`，固定 1536 B，magic `KP01`；格式见 `docs/metrics.md` 第 4 节。
- `report.html` 是按需生成的离线快照，删除不影响历史数据。
- `health.log` 只在钩子自动恢复时出现，内容为时间、估算缺口、累计次数，**不含任何按键内容**。
- `export.csv` 包含磁盘上全部历史，不受报告 366 天内嵌上限约束。
- 默认保留 90 天（`--keep-days`，0 表示不清理）。

## 7. 已知限制（发布版如实告知）

- **统计为近似值**：以管理员权限运行的窗口输入收不到（UIPI）；锁屏、UAC 安全桌面同理。只有连续 30 秒「有输入且采集不到」才判定为中断并重挂，短暂切换窗口或锁屏解锁不会误报。
- **钩子掉线有确认延迟**：心跳每 2 秒检测、需连续 30 秒静默才确认，窗口内的输入已丢失；报告顶部会标注该时段为近似值。
- **长按连发固定过滤**，v1 没有开关。
- **滚轮**按事件计 1 格，不等于实际滚动行数。
- **输入法组合过程**中的系统按键会被计入，与「实际打字数」不是一回事；今日速写的作品换算只是趣味粗估。
- **界面**借用系统默认浏览器展示 HTML，不是原生窗口。
- **本地看板是可选的实时页**：默认关闭，页面不落盘，关闭全部页面 30 秒后自动停止，之后无法再打开；需要长期留档请用托盘「统计报告 → 打开（离线快照）」。
- **体积与内存**：引入标准库 `net/http` 后单 exe 约 6.3 MiB；勾选看板时常驻内存高于默认路径，不勾选时不启动本地服务。
- **无开机自启**，每次开机需手动启动。

## 8. 升级与卸载

升级：退出托盘 → 用新 exe 覆盖旧文件。`.bin` 格式带 `KP01` magic 与 version 字段，v1 内向前兼容，历史数据不会清空。

卸载：

1. 右键托盘 → 退出（或直接结束 `keypulse.exe`）。
2. 删除 `keypulse.exe` / `keypulse-cli.exe`。
3. 可选：删除 `%LOCALAPPDATA%\KeyPulse\` 清除全部历史。
4. 若曾手动加过 Defender 排除项，记得一并移除。

无残留注册表项、无计划任务、无开机启动项。

## 9. 回滚

若是从旧版升级后异常：退出程序，换回旧 exe 即可。`.bin` 结构未变；但旧版会把 `health.log` 里历史误报行重新计入提示条（新版已忽略 `reason=input_without_callback`），需要清爽可自行归档该日志。若报告页显示异常，删掉 `report.html` 后重新双击托盘生成。已公开的 tag / Release 不删除；构建包有问题就发新的 PATCH，并在 Release 里注明勿用旧版本。

## 10. 版本与 GitHub Release

完整规则与 13 步发版 SOP 见 `docs/versioning.md`，要点：

| 对象 | 规范 |
|---|---|
| Git tag | `v<版本>`，annotated tag，例如 `v1.1.0` |
| Release 标题 | `v<版本>`，可加一句主题 |
| Release 正文 | 取自 `CHANGELOG.md` 对应版本 + 适用系统 + 校验方式 + 已知限制 + Defender 提示 + 「升级不清空数据」 |
| 上传资产 | `KeyPulse-<版本>-win-x64.exe`、`KeyPulse-<版本>-win-x64.zip`、`SHA256SUMS.txt` |
| latest | 只给稳定版；`-rc.N` / `-beta.N` 必须勾选 pre-release |

**主版本克制**：`1.x` 长期演进，新能力走 `1.x.0`、修复走 `1.1.x`，只有破坏兼容且无法自动迁移才允许 `2.0.0`。发布前必须跑 `bash scripts/check-version.sh`，它会拒绝重复 tag 与文档里写死的旧版本号。

Release 正文示例骨架：

```markdown
## 变更
（粘贴 CHANGELOG.md 对应版本内容）

## 下载与校验
- KeyPulse-<版本>-win-x64.exe：免解压，双击即用（SHA256 填打包脚本输出值，与包内 keypulse.exe 同一字节）
- KeyPulse-<版本>-win-x64.zip：解压即用，含两版 exe、示例报告与文档（SHA256 填打包脚本输出值）
- SHA256SUMS.txt：包内全部文件的校验和，Git Bash `sha256sum -c SHA256SUMS.txt`

适用 Windows 10/11 x64；构建环境 Go 1.25 / CGO_ENABLED=0。
升级只需退出托盘后覆盖 exe，历史数据不会清空。
首次运行若被 Defender 拦截，见包内 docs/release.md 第 5 节。
```
