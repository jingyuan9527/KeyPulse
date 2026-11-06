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
- [ ] 用户退出当前运行的 `keypulse.exe`（PID 3468，v1.0.0 之前的 11:05 构建）后，从 dist 启动新版或替换根目录 exe；根目录 exe 本轮**未覆盖**

## 开源发布与版本治理（2026-11-06）

- [x] 重写 `README.md` 面向公开仓库：隐私声明前置、三视图、口径摘要、版本规则、开发命令
- [x] `docs/versioning.md`：SemVer 升位决策树、**主版本克制清单**、v1 兼容承诺、13 步发版 SOP、tag / Release / 资产命名
- [x] `CHANGELOG.md`：Keep a Changelog 中文版，含 1.0.0 完整条目与常驻 Unreleased
- [x] `scripts/check-version.sh`：VERSION 格式 / CHANGELOG 条目 / tag 重复 / 文档写死版本号校验
- [x] 构建脚本去掉本机硬编码路径，改为 `.env.local` 可选覆盖；新增 `.env.local.example`
- [x] `.gitignore` / `CONTRIBUTING.md`（含不可协商的隐私红线）
- [x] `docs/metrics.md` / `docs/release.md` 去掉写死版本号；release 新增第 10 节 GitHub Release 规范
- [x] `scripts/package-release.sh` 增加版本校验、zip 打包、动态 SHA256 清单
- [ ] 🚧 确认仓库名、许可证、提交身份后：`git init` → 中文提交 → 公开仓库 → push → `v1.0.0` tag
- [ ] 创建 GitHub Release `v1.0.0`，上传 `KeyPulse-1.0.0-win-x64.zip` 与 `SHA256SUMS.txt`
- [ ] 重跑 `package-release.sh` 生成含 LICENSE 的最终发布包并复核校验和

## 已交付的 v1 能力

- [x] 双低层钩子 + 独立 OS 线程消息循环 + 回调零分配
- [x] 按天 1536 B 二进制快照 + 内存脏位图增量写盘 + 空闲零 IO
- [x] 托盘常驻、程序化两态图标、暂停/导出/退出
- [x] 离线单文件 HTML 报告：单日回看 / 周报同期对比 / 总览日历热力
- [x] 今日速写（数据驱动文案）、托盘图标、报告 10 分钟保活自动刷新
- [x] P0 钩子心跳检测与自动重挂、`health.log` 数据缺口标注

## v2 待办

- [ ] Wails 原生窗口（当前借用系统默认浏览器展示 HTML）
- [ ] 长按连发是否计入的设置开关（v1 固定过滤）
- [ ] 组合键白名单计数（Ctrl+C / Alt+Tab 等，只存计数不存序列）
- [ ] 开机自启选项（v1 不写任何注册表启动项）
- [ ] 16×16 图标实机目视复核（当前按系统尺寸绘制，已完成 DPI 尺寸适配）
- [ ] 完整托盘/钩子链路的自动化实机回归（当前依赖人工自测清单）
