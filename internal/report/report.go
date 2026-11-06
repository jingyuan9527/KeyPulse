// Package report 把全部历史统计渲染成一份完全离线的单文件 HTML 报告。
// 数据以紧凑 JSON 一次性内嵌，页面内的单日 / 周报 / 总览切换由本地 JS 完成，
// 不引 CDN、不联网、不发起请求、切换视图不产生额外磁盘 IO。
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

// Render 生成 report.html 并返回绝对路径。
// 先在内存里拼好整份文档，再一次 WriteFile：页面每 60 秒刷新、后台会重写
// 同一文件，必须把"已截断但尚未写完"的窗口压缩到一次系统调用。
func Render(dir string, days []store.DayData) (string, error) {
	if len(days) == 0 {
		return "", fmt.Errorf("没有可统计的数据")
	}
	payload, err := BuildDataset(dir, days, time.Now())
	if err != nil {
		return "", err
	}
	doc := strings.Replace(pageTemplate, dataPlaceholder, string(payload), 1)
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
