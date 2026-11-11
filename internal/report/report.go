// Package report 把全部历史统计渲染成一份完全离线的单文件 HTML 报告。
// 数据以紧凑 JSON 一次性内嵌，页面内的单日 / 周报 / 总览切换由本地 JS 完成，
// 不引 CDN、不联网、不暴露隐私、切换视图不产生额外磁盘 IO。
package report

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"keypulse/internal/store"
)

var vkNames = map[uint32]string{
	0x08: "Backspace", 0x09: "Tab", 0x0D: "Enter", 0x10: "Shift",
	0x11: "Ctrl", 0x12: "Alt", 0x13: "Pause", 0x14: "CapsLock",
	0x1B: "Esc", 0x20: "Space", 0x21: "PgUp", 0x22: "PgDn",
	0x23: "End", 0x24: "Home", 0x25: "←", 0x26: "↑", 0x27: "→", 0x28: "↓",
	0x2C: "PrtSc", 0x2D: "Ins", 0x2E: "Del", 0x5B: "Win", 0x5C: "Win右",
	0x5D: "Menu", 0x60: "Num0", 0x61: "Num1", 0x62: "Num2", 0x63: "Num3",
	0x64: "Num4", 0x65: "Num5", 0x66: "Num6", 0x67: "Num7", 0x68: "Num8",
	0x69: "Num9", 0x6A: "Num*", 0x6B: "Num+", 0x6D: "Num-", 0x6E: "Num.",
	0x6F: "Num/", 0x70: "F1", 0x71: "F2", 0x72: "F3", 0x73: "F4",
	0x74: "F5", 0x75: "F6", 0x76: "F7", 0x77: "F8", 0x78: "F9",
	0x79: "F10", 0x7A: "F11", 0x7B: "F12", 0x90: "NumLock",
	0xA0: "Shift左", 0xA1: "Shift右", 0xA2: "Ctrl左", 0xA3: "Ctrl右",
	0xA4: "Alt左", 0xA5: "Alt右", 0xBA: ";", 0xBB: "=", 0xBC: ",",
	0xBD: "-", 0xBE: ".", 0xBF: "/", 0xC0: "`", 0xDB: "[", 0xDC: "\\",
	0xDD: "]", 0xDE: "'",
}

func KeyName(vk uint32) string {
	if (vk >= 0x30 && vk <= 0x39) || (vk >= 0x41 && vk <= 0x5A) {
		return string(rune(vk))
	}
	if n, ok := vkNames[vk]; ok {
		return n
	}
	if vk == 0 {
		return "—"
	}
	return fmt.Sprintf("0x%02X", vk)
}

var keyLayout = [][]uint32{
	{0xC0, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x30, 0xBD, 0xBB, 0x08},
	{0x09, 0x51, 0x57, 0x45, 0x52, 0x54, 0x59, 0x55, 0x49, 0x4F, 0x50, 0xDB, 0xDD, 0xDC},
	{0x14, 0x41, 0x53, 0x44, 0x46, 0x47, 0x48, 0x4A, 0x4B, 0x4C, 0xBA, 0xDE, 0x0D},
	{0xA0, 0x5A, 0x58, 0x43, 0x56, 0x42, 0x4E, 0x4D, 0xBC, 0xBE, 0xBF, 0xA1},
	{0xA2, 0x5B, 0xA4, 0x20, 0xA5, 0x5D, 0xA3},
}

// ReportMaxDays 是单份 HTML 内嵌历史的上限（约一年，报告体积 ≈ 400 KB）。
// 更早的数据仍完整保留在 .bin 里，通过托盘「导出 CSV」查看。
const ReportMaxDays = 366

func comma(n uint32) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	out := ""
	for len(s) > 3 {
		out = "," + s[len(s)-3:] + out
		s = s[:len(s)-3]
	}
	return s + out
}

// Render 生成普通模式的 report.html（实时模式关闭）。
func Render(dir string, days []store.DayData) (string, error) {
	return RenderWithLive(dir, days, false)
}

const (
	metaRefreshTag = `<meta http-equiv="refresh" content="60">`
	staticNetNote  = "不联网、不上传、不暴露隐私"
	serverNetNote  = "不联网、不上传、不暴露隐私"
	staticFooter   = "页面已内嵌全部历史聚合数据，切换单日 / 周报 / 总览不产生任何额外读写；视图状态记录在地址栏，60 秒自动刷新后仍停留在当前页。报告在托盘「统计报告 → 打开（离线快照）」后的 10 分钟内随新数据自动更新，之后停止后台重写；重新点开托盘图标可立即刷新并再续 10 分钟。"
	serverFooter   = "页面由本地看板服务直出，今日数据每秒更新；只有停在周报 / 总览时才会每 60 秒重载一次。浏览器全部关闭后服务自动停止并释放端口。"
	liveFooter     = "页面已内嵌全部历史聚合数据；今日视图每秒局部更新、不整页重载，只有停在周报 / 总览时才会每 60 秒重载一次。报告在托盘「统计报告 → 打开（离线快照）」后的 10 分钟内随新数据自动更新。"
)

// safeToken 只保留十六进制字符，避免 token 被拼进 JS 字符串时引入引号或标签。
func safeToken(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// demoBadgeHTML 是 --selftest / 发布包示例报告独占的角标：观众必须一眼
// 分清演示数据与真实采集，否则与「隐私诚实」的产品叙事冲突。
const demoBadgeHTML = `<div class="demobadge" id="kpdemo">演示数据 · 由 --selftest 生成</div>`

// renderDocument 按形态替换占位符，返回完整 HTML，不写文件。
// 先替换小占位符再替换大 JSON：避免数据内容里若出现 __KP_*__ 字面量被二次替换。
func renderDocument(dir string, days []store.DayData, live, server, demo bool, token string) (string, error) {
	if len(days) == 0 {
		return "", fmt.Errorf("没有可统计的数据")
	}
	payload, err := BuildDataset(dir, days, time.Now())
	if err != nil {
		return "", err
	}
	liveFlag, serverFlag := "0", "0"
	if live {
		liveFlag = "1"
	}
	if server {
		serverFlag = "1"
	}
	refresh, netNote, footer := metaRefreshTag, staticNetNote, staticFooter
	if server {
		refresh, netNote, footer = "", serverNetNote, serverFooter
	} else if live {
		// 今日视图由 live.js 局部更新，不能无条件整页重载；
		// 改由页面 JS 只在周报 / 总览时定时重载。
		refresh, footer = "", liveFooter
	}
	// 角标是模板静态层元素（不在 <main id="view"> 内）：实时模式的 applyLive
	// 只替换视图区 innerHTML，60 秒整页刷新重读的也是同一份文件，角标不会消失。
	demoHTML := ""
	if demo {
		demoHTML = demoBadgeHTML
	}
	doc := pageTemplate
	for _, kv := range [][2]string{
		{refreshPlaceholder, refresh},
		{netNotePlaceholder, netNote},
		{footerNotePlaceholder, footer},
		{livePlaceholder, liveFlag},
		{serverPlaceholder, serverFlag},
		{tokenPlaceholder, safeToken(token)},
		{demoPlaceholder, demoHTML},
	} {
		doc = strings.Replace(doc, kv[0], kv[1], 1)
	}
	return strings.Replace(doc, dataPlaceholder, string(payload), 1), nil
}

// RenderServer 生成本地看板页：只返回字符串、不写任何文件。
// 页面用 SSE 收数据，页面内可一键开关实时推送。
func RenderServer(dir string, days []store.DayData, token string) (string, error) {
	return renderDocument(dir, days, false, true, false, token)
}

// RenderDemo 生成带「演示数据」角标的 report.html（demo=true 路径），
// 仅供 --selftest 与发布包示例报告使用；真实数据路径必须走
// Render / RenderWithLive，页面上不会出现任何演示标记。
func RenderDemo(dir string, days []store.DayData) (string, error) {
	doc, err := renderDocument(dir, days, false, false, true, "")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "report.html")
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path, nil
	}
	return abs, nil
}

// RenderWithLive 生成 report.html 并返回绝对路径。
// live=true 时注入实时模式标志，页面每秒从同目录 live.js 读取今日快照并
// 局部更新；普通模式下一行轮询代码都不会执行。
// 先在内存里拼好整份文档，再一次 WriteFile：页面会定时刷新、后台会重写
// 同一文件，必须把"已截断但尚未写完"的窗口压缩到一次系统调用。
func RenderWithLive(dir string, days []store.DayData, live bool) (string, error) {
	doc, err := renderDocument(dir, days, live, false, false, "")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "report.html")
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path, nil
	}
	return abs, nil
}
